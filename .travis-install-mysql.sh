#!/bin/sh

MYSQL_APT_CONFIG="mysql-apt-config_0.8.6-1_all.deb"

echo "Installing MySQL ${MYSQL_VERSION}..."
echo mysql-apt-config mysql-apt-config/select-server select mysql-${MYSQL_VERSION} | sudo debconf-set-selections
wget http://dev.mysql.com/get/${MYSQL_APT_CONFIG}
sudo dpkg --install ${MYSQL_APT_CONFIG}
sudo apt-get update -q
sudo apt-get install -q -y -o Dpkg::Options::=--force-confnew mysql-server
sudo mysql_upgrade
