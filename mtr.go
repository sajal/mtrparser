package mtrparser

import (
	"bytes"
	"errors"
	"github.com/abh/geoip"
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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

func (hop *MtrHop) Summarize(count int, gia *geoip.GeoIP) {
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
	if gia != nil {
		hop.ResolveASN(gia)
	}
}

func (hop *MtrHop) ResolveIPs() {
	hop.Host = make([]string, len(hop.IP))
	for idx, ip := range hop.IP {
		addr, err := net.LookupAddr(ip)
		if err == nil {
			//log.Println(addr[0], err)
			if len(addr) > 0 {
				hop.Host[idx] = addr[0]
			}
		}
	}
}

func getasn(ip string, gia *geoip.GeoIP) string {
	asntmp, _ := gia.GetName(ip)
	if asntmp != "" {
		splitted := strings.SplitN(asntmp, " ", 2)
		if len(splitted) == 2 {
			return splitted[0]
		}
	}
	return ""
}

func (hop *MtrHop) ResolveASN(gia *geoip.GeoIP) {
	hop.ASN = make([]string, len(hop.IP))
	for idx, ip := range hop.IP {
		//TODO...
		hop.ASN[idx] = getasn(ip, gia)
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

func (result *MTROutPut) Summarize(count int, gia *geoip.GeoIP) {
	for _, hop := range result.Hops {
		hop.Summarize(count, gia)
	}
}

//raw is the output from mtr command, count is the -c argument, default 10 in mtr
func NewMTROutPut(raw, target string, count int) (*MTROutPut, error) {
	//last hop comes in multiple times... https://github.com/traviscross/mtr/blob/master/FORMATS
	out := &MTROutPut{}
	out.Target = target
	rawhops := make([]rawhop, 0)
	//Store each line of output in rawhop structure
	for _, line := range strings.Split(raw, "\n") {
		things := strings.Split(line, " ")
		if len(things) == 3 {
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
	//Validate r.Target before sending
	tgt := strings.Trim(target, "\n \r") //Trim whitespace
	if strings.Contains(tgt, " ") {      //Ensure it doesnt contain space
		return nil, errors.New("Invalid hostname")
	}
	if strings.HasPrefix(tgt, "-") { //Ensure it doesnt start with -
		return nil, errors.New("Invalid hostname")
	}
	addrs, err := net.LookupIP(tgt)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("Host not found")
	}
	var cmd *exec.Cmd
	realtgt := ""
	switch IPv {
	case "4":
		for _, ip := range addrs {
			i := ip.To4()
			if i != nil {
				realtgt = i.String()
			}
		}
		if realtgt == "" {
			return nil, errors.New("No IPv4 address found")
		}
		cmd = exec.Command("mtr", "--raw", "-n", "-c", "10", "-4", realtgt)
	case "6":
		for _, ip := range addrs {
			i := ip.To16()
			if i != nil {
				realtgt = i.String()
			}
		}
		if realtgt == "" {
			return nil, errors.New("No IPv4 address found")
		}
		cmd = exec.Command("mtr", "--raw", "-n", "-c", "10", "-6", realtgt)
	default:
		realtgt = addrs[0].String() //Choose first addr..
		cmd = exec.Command("mtr", "--raw", "-n", "-c", "10", realtgt)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, errors.New(stderr.String())
	}
	return NewMTROutPut(out.String(), realtgt, 10)
}
