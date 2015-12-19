#!/bin/bash -e
PACKAGE=trivialdns
VERSION=1.1
REVISION=1
MAINTAINER="Alexey Kryuchkov"
EMAIL="alexey.kruchkov@gmail.com"
FULLNAME="${PACKAGE}_$VERSION-$REVISION"

rm -rf $FULLNAME
mkdir $FULLNAME

mkdir -p $FULLNAME/usr/local/bin
cp ../trivialdns $FULLNAME/usr/local/bin/
mkdir -p $FULLNAME/etc/init
cp trivialdns_init.conf $FULLNAME/etc/init/trivialdns.conf
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
Depends: upstart (>= 1.12.1)
Maintainer: $MAINTAINER <$EMAIL>
Description: Trivial DNS server
 DNS proxy server with local hostname database editable
 through web interface.
EOF

dpkg-deb --build $FULLNAME
