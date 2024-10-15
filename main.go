package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"

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

var certs = map[string]struct {
	certFile string
	keyFile  string
}{
	"Alice":    {"Alice.crt", "Alice.key"},
	"Bob":      {"Bob.crt", "Bob.key"},
	"Charlie":  {"Charlie.crt", "Charlie.key"},
	"Hospital": {"Hospital.crt", "Hospital.key"},
}

type peer struct {
	message.UnimplementedServiceServer
	id             int32
	name           string
	clients        map[int32]message.ServiceClient
	ctx            context.Context
	receivedChunks []int
}

var hospitalId int32 = 5000

func main() {
	log.SetFlags(log.Lshortfile)
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + hospitalId //clients are 5001 5002 5003

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:             ownPort,
		clients:        make(map[int32]message.ServiceClient),
		receivedChunks: []int{},
		ctx:            ctx,
	}

	//set up server
	list, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", ownPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	serverCert, err := credentials.NewServerTLSFromFile("certificate/server.crt", "certificate/priv.key")
	if err != nil {
		log.Fatalln("failed to create cert", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(serverCert))
	message.RegisterServiceServer(grpcServer, p)

	// start the server
	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("failed to serve %v", err)
		}
	}()

	//Dial the other peers
	for i := 0; i <= 3; i++ {
		port := hospitalId + int32(i)

		if port == ownPort {
			continue
		}

		//Set up client connections
		clientCert, err := credentials.NewClientTLSFromFile("certificate/server.crt", "")
		if err != nil {
			log.Fatalln("failed to create cert", err)
		}

		fmt.Printf("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithTransportCredentials(clientCert), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect: %s", err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c
		fmt.Printf("%v", p.clients)
	}
	scanner := bufio.NewScanner(os.Stdin)
	if ownPort != hospitalId {
		fmt.Print("Enter a number between 0 and 1 000 000 to share it secretly with the other peers.\nNumber: ")
		for scanner.Scan() {
			secret, _ := strconv.ParseInt(scanner.Text(), 10, 32)
			p.ShareDataChunks(int(secret))
		}
	} else {
		fmt.Print("Waiting for data from peers...\nwrite 'quit' to end me\n")
		for scanner.Scan() {
			if scanner.Text() == "quit" {
				return
			}
		}
	}
}
func (p *peer) ShareDataChunks(secret int) {

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
