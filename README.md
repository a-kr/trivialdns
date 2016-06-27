# trivialdns
Simple DNS proxy server with /etc/hosts-like local database editable via web interface. Written in Go.

* Handles `A` queries, everything else is proxied to upstream servers
* Anything not found in local database is also proxied
* Can respond with static IP address for a given domain (configured like in /etc/hosts)
* Can respond with IP address of _another_ domain (thus enabling something like DNS redirection:
  client asks for foo.com and receives the address of bar.com)

## How to run

Create a configuration file with IP addresses of upstream DNS servers (at least one address is required):
```
mkdir -p /etc/trivialdns
echo "8.8.8.8" > /etc/trivialdns/nameservers
echo "8.8.4.4" >> /etc/trivialdns/nameservers
```

DNS queries for hostnames not found in local database will be proxied to these servers.

Fetch and compile the code:
```
git clone https://github.com/a-kr/trivialdns
cd trivialdns
make
```

Run:
```
sudo ./trivialdns
```

## /etc/trivialdns/hosts

This file stores a local DNS database for `trivialdns`. File uses the following format:
```
# anything after `#` is a comment

# for A-queries about example.com, trivialdns will respond with address 2.4.3.1
example.com   2.4.3.1

# wildcard entries also work
*.example.com    1.2.3.4

# for A-queries about foo.com, trivialdns will respond with address of bar.com
foo.com       bar.com
# (bar.com is resolved every time foo.com is requested, not just once)
```

/etc/trivialdns/hosts is read once on server startup.

## Web interface

Web interface is available on port `8053`, a simple page which provides an editor for
`/etc/trivialdns/hosts` file. When changes are submitted, they are applied immediately
(no need to restart the server).

## Debian Packaging

```
make pkg
# package/trivialdns_1.0-1.deb file is produced
```
