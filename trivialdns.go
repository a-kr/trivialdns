package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	runtime_debug "runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	CommonTimeout     = 3 * time.Second
	HostsConfig       = "/etc/trivialdns/hosts"
	NameserversConfig = "/etc/trivialdns/nameservers"
)

var (
	debugMode     = flag.Bool("debug", false, "print debug messages")
	webListenAddr = flag.String("web", ":8053", "setup web interface on this port")
)

func debug(fmt string, args ...interface{}) {
	if *debugMode {
		log.Printf(fmt, args...)
	}
}

func panicCatcher() {
	if r := recover(); r != nil {
		runtime_debug.PrintStack()
		log.Printf("PANIC: %s", r)
	}
}

func looksLikeDomainName(s string) bool {
	// domain names should have alphabet characters in them
	return strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz")
}

type TrivialDnsServer struct {
	Servers  []string
	Database map[string]string

	stats     map[string]int
	statsLock sync.Mutex
}

func (self *TrivialDnsServer) Count(statname string) {
	self.statsLock.Lock()
	self.stats[statname] += 1
	self.statsLock.Unlock()
}

type StatTuple struct {
	Key   string
	Value int
}

func (self *TrivialDnsServer) GetStats() []StatTuple {
	result := []StatTuple{}
	keys := []string{}
	self.statsLock.Lock()
	for k, _ := range self.stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		result = append(result, StatTuple{k, self.stats[k]})
	}
	self.statsLock.Unlock()
	return result
}

func (self *TrivialDnsServer) refuse(w dns.ResponseWriter, req *dns.Msg) {
	self.Count("refusals")
	self.refuseWithCode(w, req, dns.RcodeRefused)
}

func (self *TrivialDnsServer) refuseOnPanic(w dns.ResponseWriter, req *dns.Msg) {
	self.Count("panic_refusals")
	self.refuseWithCode(w, req, dns.RcodeServerFailure)
}

func (self *TrivialDnsServer) refuseWithCode(w dns.ResponseWriter, req *dns.Msg, code int) {
	m := new(dns.Msg)
	for _, r := range req.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			m.SetEdns0(4096, r.(*dns.OPT).Do())
		}
	}
	m.SetRcode(req, code)
	w.WriteMsg(m)
}

func (self *TrivialDnsServer) getSingleSimpleQuestion(m *dns.Msg) *dns.Question {
	if len(m.Question) != 1 {
		return nil
	}
	q := m.Question[0]
	if q.Qtype != dns.TypeA || q.Qclass != dns.ClassINET {
		return nil
	}
	return &q
}

func (self *TrivialDnsServer) getSingleSimpleAnswer(m *dns.Msg) *net.IP {
	if len(m.Answer) != 1 {
		return nil
	}
	a, ok := m.Answer[0].(*dns.A)
	if !ok {
		return nil
	}
	if a.Hdr.Rrtype != dns.TypeA || a.Hdr.Class != dns.ClassINET {
		return nil
	}
	return &a.A
}

func (self *TrivialDnsServer) respondSuccessively(w dns.ResponseWriter, r *dns.Msg, addr net.IP) {
	q := self.getSingleSimpleQuestion(r)
	a := &dns.A{
		Hdr: dns.RR_Header{
			Name:     q.Name,
			Rrtype:   dns.TypeA,
			Class:    dns.ClassINET,
			Ttl:      30,
			Rdlength: 4,
		},
		A: addr,
	}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Answer = []dns.RR{a}
	m.Authoritative = false
	w.WriteMsg(m)
}

func (self *TrivialDnsServer) tryAnswer(w dns.ResponseWriter, r *dns.Msg) bool {
	q := self.getSingleSimpleQuestion(r)
	if q == nil {
		return false
	}
	name := strings.TrimSuffix(q.Name, ".")
	if name == "panic.com" {
		panic("panic on request for panic.com")
	}
	value, ok := self.Database[name]
	if !ok {
		debug("%s: %s not found in local database", w.RemoteAddr(), name)
		parts := strings.Split(name, ".")
		b_found := false
		for len(parts) >= 2 {
			parts[0] = "*"
			wildcard_name := strings.Join(parts, ".")
			value, ok = self.Database[wildcard_name]
			if !ok {
				parts = parts[1:]
				continue
			} else {
				b_found = true
				break
			}
		}
		if !b_found {
			return false
		}
	}
	if looksLikeDomainName(value) {
		debug("%s: %s found in database -> redirect to %s", w.RemoteAddr(), name, value)
		self.redirectQuery(w, r, value)
		return true
	}

	debug("%s: %s found in database -> %s", w.RemoteAddr(), name, value)
	addr := net.ParseIP(value)
	self.Count("local_responses")
	self.respondSuccessively(w, r, addr)
	return true
}

func (self *TrivialDnsServer) redirectQuery(w dns.ResponseWriter, r *dns.Msg, newName string) {
	self.Count("redirected_requests")
	if !strings.HasSuffix(newName, ".") {
		newName = newName + "."
	}

	newR := new(dns.Msg)
	newR.SetQuestion(dns.Fqdn(newName), dns.TypeA)

	if response, _, err := self.exchangeWithUpstream(newR); err == nil {
		ip := self.getSingleSimpleAnswer(response)
		if ip == nil {
			debug("%s redirect to %s yielded no answer", w.RemoteAddr(), newName)
			self.Count("redirected_nowhere")
			self.refuse(w, r)
			return
		}
		self.Count("redirected_successively")
		self.respondSuccessively(w, r, *ip)
	} else {
		self.Count("upstream_errors")
		self.refuse(w, r)
		log.Printf("%s: error: %s", w.RemoteAddr(), err)
	}
}

func (self *TrivialDnsServer) proxyToUpstream(w dns.ResponseWriter, r *dns.Msg) {
	self.Count("proxied_requests")
	if response, _, err := self.exchangeWithUpstream(r); err == nil {
		if len(response.Answer) == 0 {
			self.Count("proxied_refusals")
		}
		w.WriteMsg(response)
	} else {
		self.Count("upstream_errors")
		self.refuse(w, r)
		log.Printf("%s: error: %s", w.RemoteAddr(), err)
	}
}

func (self *TrivialDnsServer) exchangeWithUpstream(r *dns.Msg) (*dns.Msg, time.Duration, error) {
	self.Count("upstream_queries")
	c := new(dns.Client)

	c.ReadTimeout = CommonTimeout
	c.WriteTimeout = CommonTimeout

	i := rand.Intn(len(self.Servers))
	upstreamServer := self.Servers[i]

	response, rtt, err := c.Exchange(r, upstreamServer)
	return response, rtt, err
}

func (self *TrivialDnsServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	defer func() {
		if err := recover(); err != nil {
			runtime_debug.PrintStack()
			log.Printf("PANIC: %s", err)
			self.refuseOnPanic(w, r)
		}
	}()

	self.Count("requests")
	if self.tryAnswer(w, r) {
		return
	}
	self.proxyToUpstream(w, r)
}

func readAllLinesFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}

func readUpstreamServersFromConfig() []string {
	config := "/etc/trivialdns/nameservers"
	lines, err := readAllLinesFromFile(config)
	if err != nil {
		log.Fatalf("Failed to read %s: %s; exiting", config, err)
	}
	servers := []string{}
	for _, line := range lines {
		if !strings.Contains(line, ":") {
			line = line + ":53"
		}
		servers = append(servers, line)
	}
	return servers
}

func readDatabaseFromConfig() map[string]string {
	db := make(map[string]string)
	lines, err := readAllLinesFromFile(HostsConfig)
	if err != nil {
		log.Printf("Failed to read %s: %s; starting with empty database", HostsConfig, err)
		ioutil.WriteFile(HostsConfig, []byte{}, 0644)
		return db
	}
	for _, sourceLine := range lines {
		// remove comments
		parts := strings.Split(sourceLine, "#")
		line := strings.TrimSpace(parts[0])
		if line == "" {
			continue
		}
		parts = strings.Fields(line)
		if len(parts) != 2 {
			log.Printf("Suspicious line in config, ignoring: %s", sourceLine)
			continue
		}
		db[parts[0]] = parts[1]
	}
	return db
}

func (self *TrivialDnsServer) WebIndexPage(w http.ResponseWriter, r *http.Request) {
	defer panicCatcher()
	fmt.Fprintf(w, "<!DOCTYPE html>\n")
	fmt.Fprintf(w, "<html>\n")
	fmt.Fprintf(w, "<head>\n")
	fmt.Fprintf(w, "<title>TrivialDNS</title>\n")
	fmt.Fprintf(w, "</head>\n")
	fmt.Fprintf(w, "<body>\n")
	fmt.Fprintf(w, "<h1>TrivialDNS</h1>\n")
	fmt.Fprintf(w, "<h2>Names</h2>\n")
	fmt.Fprintf(w, "Line format: <code>{domain_name} {ip_address|domain_name_to_redirect_to}</code>\n")
	fmt.Fprintf(w, "<form method=\"POST\" action=\"/save_hosts\">\n")
	fmt.Fprintf(w, "<textarea name=\"hosts\" cols=\"80\" rows=\"25\">\n")
	lines, _ := readAllLinesFromFile(HostsConfig)
	for _, line := range lines {
		fmt.Fprintf(w, "%s\n", line)
	}
	fmt.Fprintf(w, "</textarea>\n")
	fmt.Fprintf(w, "<br/>\n")
	fmt.Fprintf(w, "<input type=\"submit\" value=\"Update\">\n")
	fmt.Fprintf(w, "</form>\n")

	fmt.Fprintf(w, "<h2>Stats</h2>\n")
	fmt.Fprintf(w, "<table>\n")
	for _, st := range self.GetStats() {
		fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", st.Key, st.Value)
	}
	fmt.Fprintf(w, "</table>\n")
	fmt.Fprintf(w, "</body>\n")
	fmt.Fprintf(w, "</html>\n")
}

func (self *TrivialDnsServer) WebSaveHosts(w http.ResponseWriter, r *http.Request) {
	defer panicCatcher()
	if r.Method != "POST" {
		http.Error(w, "Expected POST query", 400)
		return
	}
	hosts := r.FormValue("hosts")
	if hosts == "" {
		http.Error(w, "`hosts` parameter is required", 400)
		return
	}
	err := ioutil.WriteFile(HostsConfig, []byte(hosts), 0644)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not save hosts database: %s", err), 400)
		return
	}

	self.Database = readDatabaseFromConfig()
	log.Printf("Database updated: %v", self.Database)
	http.Redirect(w, r, "/", 302)
}

func main() {
	flag.Parse()

	tdns := &TrivialDnsServer{
		Servers:  readUpstreamServersFromConfig(),
		Database: readDatabaseFromConfig(),
		stats:    make(map[string]int),
	}

	log.Printf("Starting with nameservers %v", tdns.Servers)
	log.Printf("Starting with database %v", tdns.Database)

	addr := ":53"

	go func() {
		if err := dns.ListenAndServe(addr, "udp", tdns); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		if err := dns.ListenAndServe(addr, "tcp", tdns); err != nil {
			log.Fatal(err)
		}
	}()

	log.Printf("DNS server started")
	log.Printf("Starting web interface on %s", *webListenAddr)
	http.HandleFunc("/", tdns.WebIndexPage)
	http.HandleFunc("/save_hosts", tdns.WebSaveHosts)
	http.ListenAndServe(*webListenAddr, nil)
}
