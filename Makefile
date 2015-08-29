trivialdns: trivialdns.go .depends
	go build

pkg: trivialdns
	cd package && ./package.sh


.depends:
	go get "github.com/miekg/dns"
	touch .depends

clean:
	rm trivialdns
	rm .depends

