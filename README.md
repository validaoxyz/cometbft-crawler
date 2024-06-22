# cometbft-crawler

This is a small go program that can be used to crawl cometbft-based blockchains to discover their p2p landscape. Given a set of seed nodes (just initial rpcs; not seeds in the sense of cometbft seed nodes), it queries them for their peers, and recursively these for their peers, until a full map has been collected.

The result is a CSV file with IP address, node moniker, and cometbft version.

## How to run
To build:
```bash
go build crawler.go
```
To run:
```
./crawler.go --seeds "<seed-rpcs>" --timeout <seconds> --output <output-file>
```
