class AddExpressionIndexToLinks < ActiveRecord::Migration
  def up
    ActiveRecord::Base.connection.execute 'CREATE INDEX index_links_on_substring_head_uuid on links (substring(head_uuid, 7, 5))'
    ActiveRecord::Base.connection.execute 'CREATE INDEX index_links_on_substring_tail_uuid on links (substring(tail_uuid, 7, 5))'
  end

  def down
    ActiveRecord::Base.connection.execute 'DROP INDEX index_links_on_substring_head_uuid'
    ActiveRecord::Base.connection.execute 'DROP INDEX index_links_on_substring_tail_uuid'
  end
end
