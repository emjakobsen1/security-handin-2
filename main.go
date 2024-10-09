package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	message "github.com/emjakobsen1/dsysmutex/proto"
	"google.golang.org/grpc"
)

type peer struct {
	message.UnimplementedServiceServer
	id      int32
	clients map[int32]message.ServiceClient
	ctx     context.Context
}

func main() {
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 5000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:      ownPort,
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
	for i := 0; i < 3; i++ {
		port := 5000 + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		log.Printf("Attempting dial: %v\n", port)

		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("Dial failed: %s", err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c
	}

	log.Printf("Connected to all clients\n")
	time.Sleep(5 * time.Second)

	// Example of sending a message to peers periodically
	for {
		p.sendMessageToPeers()
		time.Sleep(5 * time.Second)
	}
}

func (p *peer) sendMessageToPeers() {
	log.Printf("(%v) Send | Sending message to peers", p.id)
	info := &message.Info{Id: p.id}
	for id, client := range p.clients {
		_, err := client.Request(p.ctx, info)
		if err != nil {
			log.Printf("Failed to send message to peer %v: %v", id, err)
		}
	}
}

// Simple gRPC service method to handle incoming requests
func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	log.Printf("(%v) Recv | Received message from %v", p.id, req.Id)
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
