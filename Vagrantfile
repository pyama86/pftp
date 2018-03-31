# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure("2") do |config|
  config.vm.box = "centos/7"
  config.vm.network :private_network, ip:"192.168.33.2"
  config.vm.provision "shell", inline: <<-SHELL
  yum -y install vsftpd
  sleep 3
  chkconfig vsftpd on | true
  useradd test | true
  echo -n 'test:pftp' | chpasswd
  sed -i 's/userlist_enable=YES/userlist_enable=NO/g' /etc/vsftpd/vsftpd.conf
  sed -i 's/tcp_wrappers=YES/tcp_wrappers=NO/g' /etc/vsftpd/vsftpd.conf
  if ! grep 'log_ftp_protocol=YES' '/etc/vsftpd/vsftpd.conf' >/dev/null; then
    echo 'log_ftp_protocol=YES' >> '/etc/vsftpd/vsftpd.conf'
    echo 'syslog_enable=YES' >> '/etc/vsftpd/vsftpd.conf'
  fi
  setenforce 0
  service vsftpd restart
  SHELL
end
