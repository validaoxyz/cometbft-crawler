package main

import (
    "encoding/csv"
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "strings"
    "time"
)

var (
    client *http.Client
    seen   map[string]bool
)

type StatusResponse struct {
    Result struct {
        NodeInfo struct {
            Network string `json:"network"`
        } `json:"node_info"`
    } `json:"result"`
}

type NetInfoResponse struct {
    Result struct {
        Peers []struct {
            NodeInfo struct {
                Moniker string `json:"moniker"`
                Version string `json:"version"`
                Other   struct {
                    RpcAddress string `json:"rpc_address"`
                } `json:"other"`
            } `json:"node_info"`
            RemoteIP string `json:"remote_ip"`
        } `json:"peers"`
    } `json:"result"`
}

func queryNode(url string, path string) ([]byte, error) {
    resp, err := client.Get(url + path)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    return ioutil.ReadAll(resp.Body)
}

func checkAndAddNode(network, url string, output [][]string) [][]string {
    fmt.Printf("Querying /status for URL: %s\n", url)
    body, err := queryNode(url, "/status")
    if err != nil {
        fmt.Printf("Error querying /status for %s: %v\n", url, err)
        return output
    }

    var status StatusResponse
    if err := json.Unmarshal(body, &status); err != nil {
        fmt.Printf("Error decoding status response for %s: %v\n", url, err)
        return output
    }

    if status.Result.NodeInfo.Network != network {
        fmt.Printf("Network mismatch for %s, expected %s, got %s\n", url, network, status.Result.NodeInfo.Network)
        return output
    } else {
        fmt.Printf("Network match confirmed for %s: %s\n", url, network)
    }

    fmt.Printf("Querying /net_info for URL: %s\n", url)
    body, err = queryNode(url, "/net_info")
    if err != nil {
        fmt.Printf("Error querying /net_info for %s: %v\n", url, err)
        return output
    }

    var netInfo NetInfoResponse
    if err := json.Unmarshal(body, &netInfo); err != nil {
        fmt.Printf("Error decoding net_info response for %s: %v\n", url, err)
        return output
    }

    fmt.Printf("Found %d peers for %s\n", len(netInfo.Result.Peers), url)
    for _, peer := range netInfo.Result.Peers {
        ip := peer.RemoteIP
        moniker := peer.NodeInfo.Moniker
        version := peer.NodeInfo.Version // Fetch version from the correct place
        rpcAddress := peer.NodeInfo.Other.RpcAddress

        rpcAddressParts := strings.Split(rpcAddress, ":")
        if len(rpcAddressParts) < 3 {
            fmt.Printf("Unexpected RPC address format for peer %s: %s\n", ip, rpcAddress)
            continue
        }
        rpcPort := rpcAddressParts[len(rpcAddressParts)-1]

        fullAddress := "http://" + ip + ":" + rpcPort
        fmt.Printf("Constructed URL for querying peer: %s, Moniker: %s, Version: %s\n", fullAddress, moniker, version)

        if _, exists := seen[fullAddress]; !exists {
            seen[fullAddress] = true
            output = append(output, []string{ip, moniker, version}) // Assuming connection is always successful here
            fmt.Printf("Adding new peer to output: %s, %s\n", ip, moniker)
            output = checkAndAddNode(network, fullAddress, output)
        } else {
            fmt.Printf("Peer already processed: %s\n", fullAddress)
        }
    }
    return output
}

func getNetworkIDFromSeed(seed string) (string, error) {
    body, err := queryNode(seed, "/status")
    if err != nil {
        return "", err
    }

    var status StatusResponse
    if err := json.Unmarshal(body, &status); err != nil {
        return "", err
    }

    return status.Result.NodeInfo.Network, nil
}

func getNetworkIDFromSeeds(seedNodes []string, maxRetries int, retryInterval time.Duration) (string, error) {
    var lastErr error
    for attempt := 1; attempt <= maxRetries; attempt++ {
        for _, seed := range seedNodes {
            fmt.Printf("Attempt %d: Trying to get network ID from seed node %s\n", attempt, seed)
            networkID, err := getNetworkIDFromSeed(seed)
            if err != nil {
                fmt.Printf("Failed to get network ID from seed node %s: %v\n", seed, err)
                lastErr = err
                continue
            }
            return networkID, nil
        }
        if attempt < maxRetries {
            fmt.Printf("All seed nodes failed in attempt %d. Retrying in %s...\n", attempt, retryInterval)
            time.Sleep(retryInterval)
        }
    }
    return "", fmt.Errorf("unable to get network ID from any of the provided seed nodes after %d attempts: %v", maxRetries, lastErr)
}

func main() {
    var seeds string
    var timeout int
    var outputFile string
    flag.StringVar(&seeds, "seeds", "", "Comma-separated list of seed nodes")
    flag.IntVar(&timeout, "timeout", 0, "Timeout in seconds")
    flag.StringVar(&outputFile, "output", "", "Outpute filename and path")
    flag.Parse()

    if seeds == "" || timeout == 0 || outputFile == "" {
        fmt.Println("Both --seeds and --timeout and --output flags are required")
        os.Exit(1)
    }

    seedNodes := strings.Split(seeds, ",")
    client = &http.Client{Timeout: time.Duration(timeout) * time.Second}
    seen = make(map[string]bool)

    // Define retry parameters
    maxRetries := 3
    retryInterval := 10 * time.Second

    // Determine network ID from the provided seed nodes with retries
    networkID, err := getNetworkIDFromSeeds(seedNodes, maxRetries, retryInterval)
    if err != nil {
        log.Fatalf("Failed to determine network ID: %v", err)
    }
    fmt.Printf("Using network ID: %s\n", networkID)

    var output [][]string
    for _, seed := range seedNodes {
        output = checkAndAddNode(networkID, seed, output)
    }

    file, err := os.Create(outputFile)
    if err != nil {
        log.Fatal("Cannot create file", err)
    }
    defer file.Close()

    writer := csv.NewWriter(file)
    defer writer.Flush()

    // Write the header row with category names
    if err := writer.Write([]string{"ip", "moniker", "version"}); err != nil {
        log.Fatal("Error writing header to file", err)
    }

    for _, value := range output {
        if err := writer.Write(value); err != nil {
            log.Fatalln("Error writing record to file", err)
        }
    }
    fmt.Println("Output successfully written to ", outputFile)
}
