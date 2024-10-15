package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc/credentials"

	message "github.com/emjakobsen1/security-handin-2/proto"
	"google.golang.org/grpc"
)

var nameToPort = map[string]int32{
	"Alice":    5000,
	"Bob":      5001,
	"Charlie":  5002,
	"Hospital": 5003,
}

var portToName = func() map[int32]string {
	m := make(map[int32]string)
	for name, port := range nameToPort {
		m[port] = name
	}
	return m
}()

type peer struct {
	message.UnimplementedServiceServer
	id      int32
	name    string
	clients map[int32]message.ServiceClient
	ctx     context.Context
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: go run main.go [name]")
	}

	// Get the name from the command-line arguments
	name := os.Args[1]
	ownPort, exists := nameToPort[name]
	if !exists {
		log.Fatalf("Invalid name. Use one of: Alice, Bob, Charlie, Hospital")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:      ownPort,
		name:    name, // Set the peer's name here
		clients: make(map[int32]message.ServiceClient),
		ctx:     ctx,
	}

	// Create listener on ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		log.Fatalf("Could not listen on port: %v", err)
	}

	f, _ := setLog(ownPort)
	defer f.Close()

	grpcServer := grpc.NewServer()
	message.RegisterServiceServer(grpcServer, p)

	// Start gRPC server in the background
	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Connect to other peers
	for peerName, port := range nameToPort {
		if port == ownPort {
			continue
		}

		log.Printf("[%s] Attempting dial: %s\n", p.name, peerName)

		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("[%s] Dial failed to %s: %s", p.name, peerName, err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c
	}

	log.Printf("[%s] Connected to all clients\n", p.name)
	time.Sleep(5 * time.Second)

	// Example of sending a message to peers periodically
	for {
		p.sendMessageToPeers()
		time.Sleep(5 * time.Second)
	}
}

func loadTLSCredentials(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("could not load server key pair: %s", err)
	}

	// Load CA's certificate
	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("could not read CA certificate: %s", err)
	}

	// Create a certificate pool from CA's certificate
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create the TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},        // Server's certificate
		ClientCAs:    caCertPool,                     // Trust CA's certificate
		ClientAuth:   tls.RequireAndVerifyClientCert, // Enforce mutual TLS
	}

	// Create TransportCredentials
	return credentials.NewTLS(config), nil
}

func (p *peer) sendMessageToPeers() {
	log.Printf("[%s] Send | Sending message to peers", p.name)
	info := &message.Info{Id: p.id}
	for id, client := range p.clients {
		peerName := portToName[id] // Get the peer name from the ID
		_, err := client.Request(p.ctx, info)
		if err != nil {
			log.Printf("[%s] Failed to send message to peer %s: %v", p.name, peerName, err)
		}
	}
}

// Simple gRPC service method to handle incoming requests
func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	peerName := portToName[req.Id] // Get the peer name from the request ID
	log.Printf("[%s] Recv | Received message from %s", p.name, peerName)
	return &message.Empty{}, nil
}

func setLog(port int32) (*os.File, *os.File) {
	filename := fmt.Sprintf("logs/peer(%v)-log.txt", port)
	if err := os.Truncate(filename, 0); err != nil {
		log.Printf("Failed to truncate: %v\n", err)
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetFlags(0)
	log.SetOutput(mw)

	cslog, err := os.OpenFile("logs/cs-log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("Error opening critical section log file: %v", err)
	}

	return f, cslog
}
