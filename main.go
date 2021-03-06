package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tm "github.com/buger/goterm" // needed for terminal output
	"github.com/miekg/dns"       // needed for dns queries
)

// Represents the domain that should be checked
type domainRequest struct {
	domain     string // Domain name
	recordType uint16 // record type that is requested
}

func (dr *domainRequest) SetDomain(name string) {
	dr.domain = name
}

// Represents a single DNS Server
type DNSResolver struct {
	ipaddress      string        // IP Address of the DNS Resolver
	successQueries int           // amount of successfullQueries
	errorQueries   int           // amount of QUeries with errors
	lastDelay      time.Duration // last answer delay
	bestDelay      time.Duration // best answer delay
	worstDelay     time.Duration // worst answer delay
	delaySum       time.Duration // a sum of all answer delays to calculate the average
	averageDelay   time.Duration // the average answer delay
	answers        []DNSAnswer   // slice with all DNSAnswer's for this DNS resolver
}

func (dnsr *DNSResolver) getQuerySum() int {
	return dnsr.successQueries + dnsr.errorQueries
}
func (dnsr *DNSResolver) getErrorPercentage() float64 {
	return float64(float64(dnsr.errorQueries) / float64(dnsr.getQuerySum()) * 100)
}

// Represents a single DNS query answer
type DNSAnswer struct {
	counter int           // counter
	delay   time.Duration // delay between question and answer in ms
	result  bool          // got we an valid answer without error?
	index   int           // number to reference to the DNSResolver struct entry
}

// slice with all DNS resolvers
var DNSResolvers = make([]DNSResolver, 0)

// Global counter at which query counter we are - starts at 0
var queryCounter int = 0

// TODO: set maximumHistoryLenght to console width
// Maximum lenght of the query history
var maximumHistoryLenght int = 30

// default domain that is requested - can be overwritten with argument
var domain domainRequest = domainRequest{"", dns.TypeA}

// Variables to hold command line arguments
var (
	argument_showHelp bool = false
	argument_domain   string
	argument_countMax int
)

/*************/
/* Functions */
/*************/

// Clear the terminal screen
func initTerminal() {
	tm.Clear()
}

// place cursor on the first position to rewrite the output
func initTerminalRewrite() {
	tm.MoveCursor(1, 1)
}

// Flush output to the terminal
func flushTerminal() {
	tm.Flush()
}

// Wait some time until the next queries and terminal write
func sleep() {
	// exit if the maximum is reached
	if argument_countMax != 0 && queryCounter >= argument_countMax {
		os.Exit(0)
	}

	time.Sleep(time.Second)
}

// query all DNS Resolver and handle the go threads for each resolver
func queryResolvers(resolvers *[]DNSResolver) {
	// create a buffered channel for all dns resolver to store the answer for each
	channel := make(chan DNSAnswer, len(*resolvers))

	// query each dns resolver in a own thread
	for i, resolver := range *resolvers {
		go queryResolver(channel, resolver, i)
	}

	counter := 0
	for answer := range channel {
		counter++
		if answer.result {
			(*resolvers)[answer.index].successQueries++
		} else {
			(*resolvers)[answer.index].errorQueries++
		}

		// add the answer to all dns resolver answers
		(*resolvers)[answer.index].answers = append((*resolvers)[answer.index].answers, answer)

		// set the last answer delay
		(*resolvers)[answer.index].lastDelay = answer.delay

		// set the best delay when this is the first answers
		if answer.counter == 0 {
			(*resolvers)[answer.index].bestDelay = answer.delay
		}

		// set the best and worst answer delay for this resolver
		if (*resolvers)[answer.index].bestDelay > answer.delay {
			(*resolvers)[answer.index].bestDelay = answer.delay
		}
		if (*resolvers)[answer.index].worstDelay < answer.delay {
			(*resolvers)[answer.index].worstDelay = answer.delay
		}

		// calculate the average delay
		(*resolvers)[answer.index].delaySum += answer.delay
		(*resolvers)[answer.index].averageDelay = time.Duration(int64((*resolvers)[answer.index].delaySum) / int64((*resolvers)[answer.index].getQuerySum()))

		// delete the oldest DNSResolver.answer entry when there amount exceed maximumHistoryLenght
		if len((*resolvers)[answer.index].answers) > maximumHistoryLenght {
			(*resolvers)[answer.index].answers = (*resolvers)[answer.index].answers[1:]
		}

		// close the channel when we have all answers
		if counter == len(*resolvers) {
			close(channel)
		}
	}

	// increase the global query run counter
	queryCounter++
}

// query a specific resolver
func queryResolver(ch chan DNSAnswer, resolver DNSResolver, index int) {
	c := dns.Client{}
	c.Timeout = 1 * time.Second

	m := dns.Msg{}
	m.SetQuestion(domain.domain+".", domain.recordType)
	r, t, err := c.Exchange(&m, resolver.ipaddress+":53")

	// error or empty answer
	if err != nil || len(r.Answer) == 0 {
		ch <- DNSAnswer{counter: queryCounter, delay: t, result: false, index: index}
		return
	}

	// valid answer
	ch <- DNSAnswer{counter: queryCounter, delay: t, result: true, index: index}
	return
}

// Build the history of the last DNS answers
func getQueryHistory(resolver DNSResolver) string {

	history := ""

	answerAmount := len(resolver.answers)
	for i, answer := range resolver.answers {
		// skip the first entries so that we show only the last
		if firstAnswer := answerAmount - maximumHistoryLenght; i < firstAnswer {
			continue
		}
		if answer.result {
			history += "."
		} else {
			history += tm.Color("?", tm.RED)
		}
	}

	return history
}

// Add a DNS Resolver to the global DNSResolvers configuration
func addDNSResolver(ip string) {
	// check for ipv6 ips
	if strings.Contains(ip, ":") {
		ip = "[" + ip + "]"
	}
	DNSResolvers = append(DNSResolvers, DNSResolver{
		ipaddress:    ip,
		lastDelay:    0,
		bestDelay:    0,
		worstDelay:   0,
		delaySum:     0,
		averageDelay: 0,
		answers:      make([]DNSAnswer, 0)})
}

func printHelp() {
	fmt.Println("Usage: " + os.Args[0] + " [-c <max count>] [-d <domain>] <dns resolver ip> [<dns resolver ip> ...]")
	fmt.Println()
	fmt.Println("This tool does query all given DNS servers and report the")
	fmt.Println("answer delays and show a history of the last queries.")
	fmt.Println()
	fmt.Println("Arguments:")
	flag.PrintDefaults()
}

// Configure and parse all command line flags
func parseFlags() {
	flag.BoolVar(&argument_showHelp, "help", false, "show help")
	flag.BoolVar(&argument_showHelp, "h", false, "show help")
	flag.StringVar(&argument_domain, "d", "example.com", "`domain` that should be queried")
	flag.IntVar(&argument_countMax, "c", 0, "exit after sended `count` of DNS queries")
	flag.Parse()

	// print the help
	if argument_showHelp {
		printHelp()
		os.Exit(0)
	}

	// Set the domain that is queried
	if argument_domain != "" {
		domain.SetDomain(argument_domain)
	} else {
		fmt.Println("No domain given!")
		printHelp()
		os.Exit(1)
	}

	// Add DNS resolver
	for _, resolver := range flag.Args() {
		addDNSResolver(resolver)
	}
	if len(DNSResolvers) == 0 {
		fmt.Println("No DNS resolvers given!")
		printHelp()
		os.Exit(1)
	}
}

func init() {
	parseFlags()
}

func main() {
	initTerminal()

	for {
		// Now query all configured resolvers in go thready and wait for them
		// when we need the result
		queryResolvers(&DNSResolvers)

		initTerminalRewrite()

		// new table with table header
		outputTable := tm.NewTable(0, 8, 1, ' ', 0)
		fmt.Fprintf(outputTable, "DNS Server \tSuccess \tErrors \tError %% \tLast  \tAverage  \tBest  \tWorst  -\tQueries\n")

		// build the log line for each dns resolver
		for _, resolver := range DNSResolvers {
			fmt.Fprintf(
				outputTable, "%s\t%d\t%d\t%s\t%dms\t%dms\t%dms\t%dms\t%s\n",
				resolver.ipaddress,
				resolver.successQueries,
				resolver.errorQueries,
				strconv.FormatFloat(resolver.getErrorPercentage(), 'f', 2, 64)+"%",
				resolver.lastDelay/time.Millisecond,
				resolver.averageDelay/time.Millisecond,
				resolver.bestDelay/time.Millisecond,
				resolver.worstDelay/time.Millisecond,
				getQueryHistory(resolver))
		}
		tm.Print(outputTable)

		flushTerminal()
		sleep()
	}
}
