# Copyright (C) The Arvados Authors. All rights reserved.
#
# SPDX-License-Identifier: AGPL-3.0

all: centos7/generated debian8/generated debian9/generated ubuntu1404/generated ubuntu1604/generated ubuntu1804/generated

centos7/generated: common-generated-all
	test -d centos7/generated || mkdir centos7/generated
	cp -rlt centos7/generated common-generated/*

debian8/generated: common-generated-all
	test -d debian8/generated || mkdir debian8/generated
	cp -rlt debian8/generated common-generated/*

debian9/generated: common-generated-all
	test -d debian9/generated || mkdir debian9/generated
	cp -rlt debian9/generated common-generated/*

ubuntu1404/generated: common-generated-all
	test -d ubuntu1404/generated || mkdir ubuntu1404/generated
	cp -rlt ubuntu1404/generated common-generated/*

ubuntu1604/generated: common-generated-all
	test -d ubuntu1604/generated || mkdir ubuntu1604/generated
	cp -rlt ubuntu1604/generated common-generated/*

ubuntu1804/generated: common-generated-all
	test -d ubuntu1804/generated || mkdir ubuntu1804/generated
	cp -rlt ubuntu1804/generated common-generated/*

RVMKEY1=mpapis.asc
RVMKEY2=pkuczynski.asc

common-generated-all: common-generated/$(RVMKEY1) common-generated/$(RVMKEY2)

common-generated/$(RVMKEY1): common-generated
	wget -cqO common-generated/$(RVMKEY1) https://rvm.io/mpapis.asc

common-generated/$(RVMKEY2): common-generated
	wget -cqO common-generated/$(RVMKEY2) https://rvm.io/pkuczynski.asc

common-generated:
	mkdir common-generated
