#!/bin/bash -e
PACKAGE=trivialdns
VERSION=1.1
REVISION="ubuntu-16.04"
MAINTAINER="Alexey Kryuchkov"
EMAIL="alexey.kruchkov@gmail.com"
FULLNAME="${PACKAGE}_$VERSION-$REVISION"

rm -rf $FULLNAME
mkdir $FULLNAME

mkdir -p $FULLNAME/usr/local/bin
cp ../trivialdns $FULLNAME/usr/local/bin/
mkdir -p $FULLNAME/etc/systemd/system/
mkdir -p $FULLNAME/etc/systemd/system/multi-user.target.wants/
cp trivialdns.service $FULLNAME/etc/systemd/system/trivialdns.service
ln -s  $FULLNAME/etc/systemd/system/trivialdns.service $FULLNAME/etc/systemd/system/multi-user.target.wants/trivialdns.service
mkdir -p $FULLNAME/etc/trivialdns
echo "8.8.8.8" > $FULLNAME/etc/trivialdns/nameservers
echo "8.8.4.4" >> $FULLNAME/etc/trivialdns/nameservers
echo "example.com 1.2.3.4" > $FULLNAME/etc/trivialdns/hosts
echo "redirect.example.com target.com" >> $FULLNAME/etc/trivialdns/hosts

mkdir $FULLNAME/DEBIAN

cat >$FULLNAME/DEBIAN/control << EOF
Package: $PACKAGE
Version: $VERSION-$REVISION
Section: base
Priority: optional
Architecture: amd64
Depends: systemd (>= 229-4ubuntu21)
Maintainer: $MAINTAINER <$EMAIL>
Description: Trivial DNS server
 DNS proxy server with local hostname database editable
 through web interface.
EOF

dpkg-deb --build $FULLNAME
