#!/bin/sh
v=`git branch | grep "\* " | sed -e "s/\* //g;s/adJ74//g"`
echo ${v}

rm -rf /tmp/gen_subnet_evm
mkdir -p /tmp/gen_subnet_evm
./scripts/build.sh /tmp/gen_subnet_evm/subnet-evm
cp README.md LICENSE /tmp/gen_subnet_evm/
(cd /tmp/gen_subnet_evm/; tar cvfz ../subnet_evm_${v}_openbsd_amd64.tar.gz *)
ls -l /tmp/subnet*
cp /tmp/subnet_evm_${v}_openbsd_amd64.tar.gz ~/Downloads/
