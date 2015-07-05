# mtrparser
Parsing raw mtr output in Go

[![GoDoc](https://godoc.org/github.com/sajal/mtrparser?status.svg)](https://godoc.org/github.com/sajal/mtrparser)

This library was primarily created to parse output of [mtr command](http://www.bitwizard.nl/mtr/) for [TurboBytes Pulse](https://pulse.turbobytes.com). This parses the output of mtr with `--raw` argument into Go structures. From there one can format it however they want... regardless of the mtr version running. Also included in the struct is the timing details of each ping received.

See the reference command in cmd/mtrparser.go for usage.
