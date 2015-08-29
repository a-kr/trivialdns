trivialdns: trivialdns.go .depends
	go build

.depends:
	go get "github.com/miekg/dns"
	touch .depends

clean:
	rm trivialdns
	rm .depends
