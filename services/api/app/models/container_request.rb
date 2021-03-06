# Copyright (C) The Arvados Authors. All rights reserved.
#
# SPDX-License-Identifier: AGPL-3.0

require 'whitelist_update'

class ContainerRequest < ArvadosModel
  include ArvadosModelUpdates
  include HasUuid
  include KindAndEtag
  include CommonApiTemplate
  include WhitelistUpdate

  belongs_to :container, foreign_key: :container_uuid, primary_key: :uuid
  belongs_to :requesting_container, {
               class_name: 'Container',
               foreign_key: :requesting_container_uuid,
               primary_key: :uuid,
             }

  serialize :properties, Hash
  serialize :environment, Hash
  serialize :mounts, Hash
  serialize :runtime_constraints, Hash
  serialize :command, Array
  serialize :scheduling_parameters, Hash
  serialize :secret_mounts, Hash

  before_validation :fill_field_defaults, :if => :new_record?
  before_validation :validate_runtime_constraints
  before_validation :set_default_preemptible_scheduling_parameter
  before_validation :set_container
  validates :command, :container_image, :output_path, :cwd, :presence => true
  validates :output_ttl, numericality: { only_integer: true, greater_than_or_equal_to: 0 }
  validates :priority, numericality: { only_integer: true, greater_than_or_equal_to: 0, less_than_or_equal_to: 1000 }
  validate :validate_datatypes
  validate :validate_scheduling_parameters
  validate :validate_state_change
  validate :check_update_whitelist
  validate :secret_mounts_key_conflict
  validate :validate_runtime_token
  before_save :scrub_secrets
  before_create :set_requesting_container_uuid
  before_destroy :set_priority_zero
  after_save :update_priority
  after_save :finalize_if_needed

  api_accessible :user, extend: :common do |t|
    t.add :command
    t.add :container_count
    t.add :container_count_max
    t.add :container_image
    t.add :container_uuid
    t.add :cwd
    t.add :description
    t.add :environment
    t.add :expires_at
    t.add :filters
    t.add :log_uuid
    t.add :mounts
    t.add :name
    t.add :output_name
    t.add :output_path
    t.add :output_uuid
    t.add :output_ttl
    t.add :priority
    t.add :properties
    t.add :requesting_container_uuid
    t.add :runtime_constraints
    t.add :scheduling_parameters
    t.add :state
    t.add :use_existing
  end

  # Supported states for a container request
  States =
    [
     (Uncommitted = 'Uncommitted'),
     (Committed = 'Committed'),
     (Final = 'Final'),
    ]

  State_transitions = {
    nil => [Uncommitted, Committed],
    Uncommitted => [Committed],
    Committed => [Final]
  }

  AttrsPermittedAlways = [:owner_uuid, :state, :name, :description, :properties]
  AttrsPermittedBeforeCommit = [:command, :container_count_max,
  :container_image, :cwd, :environment, :filters, :mounts,
  :output_path, :priority, :runtime_token,
  :runtime_constraints, :state, :container_uuid, :use_existing,
  :scheduling_parameters, :secret_mounts, :output_name, :output_ttl]

  def self.limit_index_columns_read
    ["mounts"]
  end

  def logged_attributes
    super.except('secret_mounts', 'runtime_token')
  end

  def state_transitions
    State_transitions
  end

  def skip_uuid_read_permission_check
    # The uuid_read_permission_check prevents users from making
    # references to objects they can't view.  However, in this case we
    # don't want to do that check since there's a circular dependency
    # where user can't view the container until the user has
    # constructed the container request that references the container.
    %w(container_uuid)
  end

  def finalize_if_needed
    if state == Committed && Container.find_by_uuid(container_uuid).final?
      reload
      act_as_system_user do
        leave_modified_by_user_alone do
          finalize!
        end
      end
    end
  end

  # Finalize the container request after the container has
  # finished/cancelled.
  def finalize!
    update_collections(container: Container.find_by_uuid(container_uuid))
    update_attributes!(state: Final)
  end

  def update_collections(container:, collections: ['log', 'output'])
    collections.each do |out_type|
      pdh = container.send(out_type)
      next if pdh.nil?
      coll_name = "Container #{out_type} for request #{uuid}"
      trash_at = nil
      if out_type == 'output'
        if self.output_name
          coll_name = self.output_name
        end
        if self.output_ttl > 0
          trash_at = db_current_time + self.output_ttl
        end
      end
      manifest = Collection.where(portable_data_hash: pdh).first.manifest_text

      coll_uuid = self.send(out_type + '_uuid')
      coll = coll_uuid.nil? ? nil : Collection.where(uuid: coll_uuid).first
      if !coll
        coll = Collection.new(
          owner_uuid: self.owner_uuid,
          name: coll_name,
          properties: {
            'type' => out_type,
            'container_request' => uuid,
          })
      end
      coll.assign_attributes(
        portable_data_hash: pdh,
        manifest_text: manifest,
        trash_at: trash_at,
        delete_at: trash_at)
      coll.save_with_unique_name!
      self.send(out_type + '_uuid=', coll.uuid)
    end
  end

  def self.full_text_searchable_columns
    super - ["mounts", "secret_mounts", "secret_mounts_md5", "runtime_token"]
  end

  protected

  def fill_field_defaults
    self.state ||= Uncommitted
    self.environment ||= {}
    self.runtime_constraints ||= {}
    self.mounts ||= {}
    self.cwd ||= "."
    self.container_count_max ||= Rails.configuration.container_count_max
    self.scheduling_parameters ||= {}
    self.output_ttl ||= 0
    self.priority ||= 0
  end

  def set_container
    if (container_uuid_changed? and
        not current_user.andand.is_admin and
        not container_uuid.nil?)
      errors.add :container_uuid, "can only be updated to nil."
      return false
    end
    if state_changed? and state == Committed and container_uuid.nil?
      self.container_uuid = Container.resolve(self).uuid
    end
    if self.container_uuid != self.container_uuid_was
      if self.container_count_changed?
        errors.add :container_count, "cannot be updated directly."
        return false
      else
        self.container_count += 1
      end
    end
  end

  def set_default_preemptible_scheduling_parameter
    c = get_requesting_container()
    if self.state == Committed
      # If preemptible instances (eg: AWS Spot Instances) are allowed,
      # ask them on child containers by default.
      if Rails.configuration.preemptible_instances and !c.nil? and
        self.scheduling_parameters['preemptible'].nil?
          self.scheduling_parameters['preemptible'] = true
      end
    end
  end

  def validate_runtime_constraints
    case self.state
    when Committed
      [['vcpus', true],
       ['ram', true],
       ['keep_cache_ram', false]].each do |k, required|
        if !required && !runtime_constraints.include?(k)
          next
        end
        v = runtime_constraints[k]
        unless (v.is_a?(Integer) && v > 0)
          errors.add(:runtime_constraints,
                     "[#{k}]=#{v.inspect} must be a positive integer")
        end
      end
    end
  end

  def validate_datatypes
    command.each do |c|
      if !c.is_a? String
        errors.add(:command, "must be an array of strings but has entry #{c.class}")
      end
    end
    environment.each do |k,v|
      if !k.is_a?(String) || !v.is_a?(String)
        errors.add(:environment, "must be an map of String to String but has entry #{k.class} to #{v.class}")
      end
    end
    [:mounts, :secret_mounts].each do |m|
      self[m].each do |k, v|
        if !k.is_a?(String) || !v.is_a?(Hash)
          errors.add(m, "must be an map of String to Hash but is has entry #{k.class} to #{v.class}")
        end
        if v["kind"].nil?
          errors.add(m, "each item must have a 'kind' field")
        end
        [[String, ["kind", "portable_data_hash", "uuid", "device_type",
                   "path", "commit", "repository_name", "git_url"]],
         [Integer, ["capacity"]]].each do |t, fields|
          fields.each do |f|
            if !v[f].nil? && !v[f].is_a?(t)
              errors.add(m, "#{k}: #{f} must be a #{t} but is #{v[f].class}")
            end
          end
        end
        ["writable", "exclude_from_output"].each do |f|
          if !v[f].nil? && !v[f].is_a?(TrueClass) && !v[f].is_a?(FalseClass)
            errors.add(m, "#{k}: #{f} must be a #{t} but is #{v[f].class}")
          end
        end
      end
    end
  end

  def validate_scheduling_parameters
    if self.state == Committed
      if scheduling_parameters.include? 'partitions' and
         (!scheduling_parameters['partitions'].is_a?(Array) ||
          scheduling_parameters['partitions'].reject{|x| !x.is_a?(String)}.size !=
            scheduling_parameters['partitions'].size)
            errors.add :scheduling_parameters, "partitions must be an array of strings"
      end
      if !Rails.configuration.preemptible_instances and scheduling_parameters['preemptible']
        errors.add :scheduling_parameters, "preemptible instances are not allowed"
      end
      if scheduling_parameters.include? 'max_run_time' and
        (!scheduling_parameters['max_run_time'].is_a?(Integer) ||
          scheduling_parameters['max_run_time'] < 0)
          errors.add :scheduling_parameters, "max_run_time must be positive integer"
      end
    end
  end

  def check_update_whitelist
    permitted = AttrsPermittedAlways.dup

    if self.new_record? || self.state_was == Uncommitted
      # Allow create-and-commit in a single operation.
      permitted.push(*AttrsPermittedBeforeCommit)
    end

    case self.state
    when Committed
      permitted.push :priority, :container_count_max, :container_uuid

      if self.container_uuid.nil?
        self.errors.add :container_uuid, "has not been resolved to a container."
      end

      if self.priority.nil?
        self.errors.add :priority, "cannot be nil"
      end

      # Allow container count to increment by 1
      if (self.container_uuid &&
          self.container_uuid != self.container_uuid_was &&
          self.container_count == 1 + (self.container_count_was || 0))
        permitted.push :container_count
      end

      if current_user.andand.is_admin
        permitted.push :log_uuid
      end

    when Final
      if self.state_was == Committed
        # "Cancel" means setting priority=0, state=Committed
        permitted.push :priority

        if current_user.andand.is_admin
          permitted.push :output_uuid, :log_uuid
        end
      end

    end

    super(permitted)
  end

  def secret_mounts_key_conflict
    secret_mounts.each do |k, v|
      if mounts.has_key?(k)
        errors.add(:secret_mounts, 'conflict with non-secret mounts')
        return false
      end
    end
  end

  def validate_runtime_token
    if !self.runtime_token.nil? && self.runtime_token_changed?
      if !runtime_token[0..2] == "v2/"
        errors.add :runtime_token, "not a v2 token"
        return
      end
      if ApiClientAuthorization.validate(token: runtime_token).nil?
        errors.add :runtime_token, "failed validation"
      end
    end
  end

  def scrub_secrets
    if self.state == Final
      self.secret_mounts = {}
      self.runtime_token = nil
    end
  end

  def update_priority
    return unless state_changed? || priority_changed? || container_uuid_changed?
    act_as_system_user do
      Container.
        where('uuid in (?)', [self.container_uuid_was, self.container_uuid].compact).
        map(&:update_priority!)
    end
  end

  def set_priority_zero
    self.update_attributes!(priority: 0) if self.state != Final
  end

  def set_requesting_container_uuid
    c = get_requesting_container()
    if !c.nil?
      self.requesting_container_uuid = c.uuid
      # Determine the priority of container request for the requesting
      # container.
      self.priority = ContainerRequest.where(container_uuid: self.requesting_container_uuid).maximum("priority") || 0
    end
  end

  def get_requesting_container
    return self.requesting_container_uuid if !self.requesting_container_uuid.nil?
    Container.for_current_token
  end
end
