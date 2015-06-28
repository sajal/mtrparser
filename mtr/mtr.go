package mtrparser

import (
	"log"
	"strconv"
	"strings"
	"time"
)

type MtrHop struct {
	IP       string
	Host     string
	Timings  []time.Duration //In Json they become nanosecond
	Avg      time.Duration
	Loss     float64
	SD       time.Duration //TODO: Calculate this
	Sent     int
	Received int
}

func (hop *MtrHop) Summarize() {
	//After the Timings block has been populated.
	hop.Sent = 10
	hop.Received = len(hop.Timings)
	for _, t := range hop.Timings {
		hop.Avg += t / time.Duration(hop.Received)
	}
	hop.Loss = (float64(hop.Sent-hop.Received) / float64(hop.Sent)) * 100
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

func NewMTROutPut(raw string) (*MTROutPut, error) {
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
				log.Fatal(err)
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
				log.Fatal(err)
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
		hop.Summarize()
	}
	return out, nil
}
