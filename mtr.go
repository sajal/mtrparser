package mtrparser

import (
	"math"
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
	IP       string
	Host     string
	Timings  []time.Duration //In Json they become nanosecond
	Avg      time.Duration
	Loss     int
	SD       time.Duration //TODO: Calculate this
	Sent     int
	Received int
	Last     time.Duration
	Best     time.Duration
	Worst    time.Duration
}

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
	hop.Loss = (100 * (hop.Sent-hop.Received)) / hop.Sent
}

type MTROutPut struct {
	Hops     []*MtrHop
	HopCount int
}

type rawhop struct {
	datatype string
	idx      int
	value    string
}

//raw is the output from mtr command, count is the -c argument, default 10 in mtr
func NewMTROutPut(raw string, count int) (*MTROutPut, error) {
	//last hop comes in multiple times... https://github.com/traviscross/mtr/blob/master/FORMATS
	out := &MTROutPut{}
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
		}
		//hop.Timings = make([]time.Duration, 0)
	}
	for _, data := range rawhops {
		switch data.datatype {
		case "h":
			out.Hops[data.idx].IP = data.value
		case "d":
			out.Hops[data.idx].Host = data.value
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
		if hop.IP != previousip {
			previousip = hop.IP
			finalidx = idx + 1
		}
	}
	out.Hops = out.Hops[0:finalidx]
	for _, hop := range out.Hops {
		hop.Summarize(count)
	}
	return out, nil
}
