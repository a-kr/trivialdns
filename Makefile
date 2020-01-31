trivialdns: trivialdns.go .depends
	go build

pkg: trivialdns
	cd package && ./package.sh
	cd package && ./package_systemd.sh


.depends:
	go get "github.com/miekg/dns"
	touch .depends

clean:
	rm trivialdns
	rm .depends

