package mtrparser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func doesipv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	localipv6 := []string{"fc00::/7", "::1/128", "fe80::/10"}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			ip6 := ipnet.IP.To16()
			ip4 := ipnet.IP.To4()
			if ip6 != nil && ip4 == nil {
				local := false
				for _, r := range localipv6 {
					_, cidr, _ := net.ParseCIDR(r)
					if cidr.Contains(ip6) {
						local = true
					}
				}
				if !local {
					return true
				}
			}
		}
	}
	return false
}

func stdDev(timings []time.Duration, avg time.Duration) time.Duration {
	//taken from https://github.com/ae6rt/golang-examples/blob/master/goeg/src/statistics_ans/statistics.go
	if len(timings) < 2 {
		return time.Duration(0)
	}
	mean := float64(avg)
	total := 0.0
	for _, t := range timings {
		number := float64(t)
		total += math.Pow(number-mean, 2)
	}
	variance := total / float64(len(timings)-1)
	std := math.Sqrt(variance)
	return time.Duration(std)
}

type MtrHop struct {
	IP       []string
	Host     []string
	ASN      []string
	Timings  []time.Duration //In Json they become nanosecond
	Avg      time.Duration
	Loss     int
	SD       time.Duration
	Sent     int
	Received int
	Last     time.Duration
	Best     time.Duration
	Worst    time.Duration
}

//Summarize calculates various statistics for each Hop
func (hop *MtrHop) Summarize(count int) {
	//After the Timings block has been populated.
	hop.Sent = count
	hop.Received = len(hop.Timings)
	if len(hop.Timings) > 0 {
		hop.Last = hop.Timings[len(hop.Timings)-1]
		hop.Best = hop.Timings[0]
		hop.Worst = hop.Timings[0]
	}
	for _, t := range hop.Timings {
		hop.Avg += t / time.Duration(hop.Received)
		if hop.Best > t {
			hop.Best = t
		}
		if hop.Worst < t {
			hop.Worst = t
		}
	}
	hop.SD = stdDev(hop.Timings, hop.Avg)
	hop.Loss = (100 * (hop.Sent - hop.Received)) / hop.Sent
	//Populate ips
	hop.ResolveIPs()
}

type lookupresult struct {
	addr []string
	err  error
}

func reverselookup(ip string) string {
	result := ""
	ch := make(chan lookupresult, 1)
	go func() {
		addr, err := net.LookupAddr(ip)
		ch <- lookupresult{addr, err}
	}()
	select {
	case res := <-ch:
		if res.err == nil {
			//log.Println(addr[0], err)
			if len(res.addr) > 0 {
				result = res.addr[0]
			}
		}
	case <-time.After(time.Second * 1):
		result = ""
	}
	return result
}

//ResolveIPs populates the DNS hostnames of the IP in each Hop.
func (hop *MtrHop) ResolveIPs() {
	hop.Host = make([]string, len(hop.IP))
	for idx, ip := range hop.IP {
		hop.Host[idx] = reverselookup(ip)
	}
}

type MTROutPut struct {
	Hops     []*MtrHop
	Target   string //Saving this FYI
	HopCount int
}

type rawhop struct {
	datatype string
	idx      int
	value    string
}

//Summarize calls Summarize on each Hop
func (result *MTROutPut) Summarize(count int) {
	for _, hop := range result.Hops {
		hop.Summarize(count)
	}
}

//Helper function to trim or pad a string
func trimpad(input string, size int) string {
	if len(input) > size {
		input = input[0:size]
	}
	return fmt.Sprintf("%[1]*[2]s ", size*-1, input)
}

//Return milliseconds as floating point from duration
func durms(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / (1000 * 1000)
}

//String returns output similar to --report option in mtr
func (result *MTROutPut) String() string {
	output := fmt.Sprintf("HOST: %sLoss%%   Snt   Last   Avg  Best  Wrst StDev", trimpad("TODO hostname", 40))
	for i, hop := range result.Hops {
		h := "???"
		if len(hop.IP) > 0 {
			//fmt.Println(hop.Host, hop.IP)
			h = hop.Host[0]
			if h == "" {
				h = hop.IP[0]
			}
		}
		output = fmt.Sprintf("%s\n%2d.|-- %s%3d.0%%   %3d  %5.1f %5.1f %5.1f %5.1f %5.1f", output, i, trimpad(h, 38), hop.Loss, hop.Sent, durms(hop.Last), durms(hop.Avg), durms(hop.Best), durms(hop.Worst), durms(hop.SD))
	}
	return output
}

//NewMTROutPut can be used to parse output of mtr --raw <target ip> .
//raw is the output from mtr command, count is the -c argument, default 10 in mtr
func NewMTROutPut(raw, target string, count int) (*MTROutPut, error) {
	//last hop comes in multiple times... https://github.com/traviscross/mtr/blob/master/FORMATS
	out := &MTROutPut{}
	out.Target = target
	rawhops := make([]rawhop, 0)
	//Store each line of output in rawhop structure
	for _, line := range strings.Split(raw, "\n") {
		things := strings.Split(line, " ")
		if len(things) == 3 || (len(things) == 4 && things[0] == "p") {
			//log.Println(things)
			idx, err := strconv.Atoi(things[1])
			if err != nil {
				return nil, err
			}
			data := rawhop{
				datatype: things[0],
				idx:      idx,
				value:    things[2],
			}
			rawhops = append(rawhops, data)
			//Number of hops = highest index+1
			if out.HopCount < (idx + 1) {
				out.HopCount = idx + 1
			}
		}
	}
	out.Hops = make([]*MtrHop, out.HopCount)
	for idx, _ := range out.Hops {
		out.Hops[idx] = &MtrHop{
			Timings: make([]time.Duration, 0),
			IP:      make([]string, 0),
			Host:    make([]string, 0),
		}
		//hop.Timings = make([]time.Duration, 0)
	}
	for _, data := range rawhops {
		switch data.datatype {
		case "h":
			out.Hops[data.idx].IP = append(out.Hops[data.idx].IP, data.value)
		//case "d":
		//Not entirely sure if multiple IPs. Better use -n in mtr and resolve later in summarize.
		//out.Hops[data.idx].Host = append(out.Hops[data.idx].Host, data.value)
		case "p":
			t, err := strconv.Atoi(data.value)
			if err != nil {
				return nil, err
			}
			out.Hops[data.idx].Timings = append(out.Hops[data.idx].Timings, time.Duration(t)*time.Microsecond)
		}
	}
	//Filter dupe last hops
	finalidx := 0
	previousip := ""
	for idx, hop := range out.Hops {
		if len(hop.IP) > 0 {
			if hop.IP[0] != previousip {
				previousip = hop.IP[0]
				finalidx = idx + 1
			}
		}
	}
	out.Hops = out.Hops[0:finalidx]
	return out, nil
}

//Execute mtr command and return parsed output
func ExecuteMTR(target string, IPv string) (*MTROutPut, error) {
	return ExecuteMTRContext(context.Background(), target, IPv)
}

//Execute mtr command and return parsed output,
//killing the process if context becomes done before command completes.
func ExecuteMTRContext(ctx context.Context, target string, IPv string) (*MTROutPut, error) {
	//Validate r.Target before sending
	tgt := strings.Trim(target, "\n \r") //Trim whitespace
	if strings.Contains(tgt, " ") {      //Ensure it doesnt contain space
		return nil, errors.New("Invalid hostname")
	}
	if strings.HasPrefix(tgt, "-") { //Ensure it doesnt start with -
		return nil, errors.New("Invalid hostname")
	}
	realtgt := ""
	parsed := net.ParseIP(tgt)
	if parsed != nil {
		realtgt = tgt
	}
	var addrs []net.IP
	var err error
	if realtgt == "" {
		addrs, err = net.LookupIP(tgt)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, errors.New("Host not found")
		}
	}
	var cmd *exec.Cmd
	/*
		if IPv == "" {
			if doesipv6() {
				IPv = "6"
			} else {
				IPv = "4"
			}
		}
	*/
	switch IPv {
	case "4":
		if realtgt == "" {
			for _, ip := range addrs {
				i := ip.To4()
				if i != nil {
					realtgt = i.String()
				}
			}
		}
		if realtgt == "" {
			return nil, errors.New("No IPv4 address found")
		}
		cmd = exec.CommandContext(ctx, "mtr", "--raw", "-n", "-c", "10", "-4", realtgt)
	case "6":
		if realtgt == "" {
			for _, ip := range addrs {
				i := ip.To16()
				if i != nil && ip.To4() == nil { //Explicitly check if its not v4
					realtgt = i.String()
				}
			}
		}
		if realtgt == "" {
			return nil, errors.New("No IPv6 address found")
		}
		cmd = exec.CommandContext(ctx, "mtr", "--raw", "-n", "-c", "10", "-6", realtgt)
	default:
		if realtgt == "" {
			if doesipv6() {
				for _, ip := range addrs {
					i := ip.To16()
					if i != nil {
						realtgt = i.String()
					}
				}
			}
			if realtgt == "" {
				for _, ip := range addrs {
					i := ip.To4()
					if i != nil {
						realtgt = i.String()
					}
				}
			}
		}
		if realtgt == "" {
			return nil, errors.New("No IP address found")
		}
		//realtgt = addrs[0].String() //Choose first addr..
		cmd = exec.CommandContext(ctx, "mtr", "--raw", "-n", "-c", "10", realtgt)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			// Process was killed by context
			return nil, ctx.Err()
		}
		// Process finished with error code
		return nil, errors.New(stderr.String())
	}
	return NewMTROutPut(out.String(), realtgt, 10)
}
