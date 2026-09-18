#!/usr/bin/env bash
#########################################
#Copyright (c) 2021 ABLECLOUD Co. Ltd.
#
#ccvm 초기화(bootstrap)하는 스크립트
#
#최초작성자 : 윤여천 책임(ycyun@ablecloud.io)
#최초작성일 : 2021-04-12
#########################################

set -x
LOGFILE="/var/log/cloud_install.log"

systemctl enable --now mysqld
DATABASE_PASSWD="Ablecloud1!"
################# firewall setting

firewall-cmd --permanent --zone=public --add-service=mysql 2>&1 | tee -a $LOGFILE
firewall-cmd --reload
firewall-cmd --list-all 2>&1 | tee -a $LOGFILE

# 라이선스 종류에 따라 설정 $1="hv" or "ablestack" or "clostack"
sh /usr/share/cloudstack-common/scripts/util/update-mold-theme-from-license.sh "$1"

# resize partition
sgdisk -e /dev/vda
parted --script /dev/vda resizepart 3 100%
pvresize /dev/vda3
lvcreate rl -n nfs --extents 100%FREE
mkfs.xfs /dev/rl/nfs
mkdir /nfs
echo  '/dev/mapper/rl-nfs /nfs                    xfs    defaults        0 0' >> /etc/fstab
echo '/nfs *(rw,no_root_squash,async)' >> /etc/exports
systemctl enable --now nfs-server.service

mkdir /nfs/primary
mkdir /nfs/secondary


################# Setting Database
mysqladmin -uroot password $DATABASE_PASSWD
systemctl enable --now mold-usage.service
cloudstack-setup-databases cloud:$DATABASE_PASSWD --deploy-as=root:$DATABASE_PASSWD  2>&1 | tee -a $LOGFILE

# 글로벌설정 DB 업데이트
global_settings=(
  "enable.vm.network.filter.allow.all.traffic=true"
)

for i in "${global_settings[@]}"; do
  IFS='=' read -r key value <<< "$i"

  if [[ -n "$key" && -n "$value" ]]; then
    mysql --user=root --password="$DATABASE_PASSWD" -e \
      "USE cloud; UPDATE configuration SET value='$value' WHERE name='$key';" \
      2>/dev/null | tee -a "$LOGFILE"
  else
    echo "잘못된 설정 항목: $i" | tee -a "$LOGFILE"
  fi
done

cloudstack-setup-management  2>&1 | tee -a $LOGFILE

systemctl enable --now mold.service

#systemvm template 등록
/usr/share/cloudstack-common/scripts/storage/secondary/cloud-install-sys-tmplt \
-m /nfs/secondary \
-f /usr/share/ablestack/systemvmtemplate-* \
-h kvm -F

# 06시 Mold 서비스 재시작 스크립트 등록
#(crontab -l 2>/dev/null; echo "0 6 * * * /usr/bin/systemctl restart mold.service") | crontab -

# ccvm 로그 정리 스크립트 등록
(crontab -l 2>/dev/null; echo "0 0 * * 7 /usr/local/sbin/ccvm_log_maintainer.sh") | crontab -

# Delete bootstrap script file
rm -rf /root/bootstrap.sh
