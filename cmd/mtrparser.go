package main

//usage: go run mtrparser.go <hostname or ip>

import (
	//"encoding/json"
	"fmt"
	//"github.com/abh/geoip"
	"github.com/sajal/mtrparser"
	"log"
	"os"
	//"strings"
)

/*
func getasnmtr(ip string, gia *geoip.GeoIP) string {
	asntmp, _ := gia.GetName(ip)
	if asntmp != "" {
		splitted := strings.SplitN(asntmp, " ", 2)
		if len(splitted) == 2 {
			return splitted[0]
		}
	}
	return ""
}

func ResolveASNMtr(hop *mtrparser.MtrHop, gia *geoip.GeoIP) {
	hop.ASN = make([]string, len(hop.IP))
	for idx, ip := range hop.IP {
		//TODO...
		hop.ASN[idx] = getasnmtr(ip, gia)
	}
}
*/

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Need mtr destination")
	}
	target := os.Args[1]
	ipv := ""
	if len(os.Args) > 2 {
		ipv = os.Args[2]
	}
	log.Println(target, ipv)
	result, err := mtrparser.ExecuteMTR(target, ipv)
	if err != nil {
		log.Fatal(err)
	}
	/*
		gia, err := geoip.OpenType(geoip.GEOIP_ASNUM_EDITION)
		if err != nil {
			log.Println(err)
			gia = nil
		}
		log.Println(gia)
	*/
	for _, hop := range result.Hops {
		hop.Summarize(10)
	}
	/*
		for _, hop := range result.Hops {
			ResolveASNMtr(hop, gia)
		}
	*/
	/*
		fmt.Println("Result")
		fmt.Println(result)
		fmt.Println("Json")
		b, err := json.Marshal(result)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(b))
		fmt.Println("Line by line")
		for idx, item := range result.Hops {
			fmt.Printf("%v : %s %s (%s) Avg: %v, Loss : %v%% Best: %v Worst: %v Last: %v stdDev: %v\n", idx+1, item.Host, item.ASN, item.IP, item.Avg, item.Loss, item.Best, item.Worst, item.Last, item.SD)
		}
	*/
	fmt.Println("mtr --report like output")
	fmt.Println(result.String())
}
