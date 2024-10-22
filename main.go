package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc/credentials"

	message "github.com/emjakobsen1/security-handin-2/proto"
	"google.golang.org/grpc"
)

/* Each peer is both a server for incoming requests and a client for sending messages */
type peer struct {
	message.UnimplementedServiceServer
	id      int32
	name    string
	clients map[int32]message.ServiceClient
	ctx     context.Context
	shares  []int32
}

/* Modulo cycle */
var R int = 20

/* Alice Bob and Charlie are 5001 5002 5003 */
var hospitalId int32 = 5000

var nameToPort = map[string]int32{
	"Hospital": hospitalId,
	"Alice":    5001,
	"Bob":      5002,
	"Charlie":  5003,
}

var portToName = func() map[int32]string {
	m := make(map[int32]string)
	for name, port := range nameToPort {
		m[port] = name
	}
	return m
}()

func main() {
	log.SetFlags(log.Lshortfile)
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + hospitalId

	secret, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf("Invalid second argument: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:      ownPort,
		clients: make(map[int32]message.ServiceClient),
		shares:  make([]int32, 0, 3),
		ctx:     ctx,
		name:    portToName[ownPort],
	}

	/* Set up the peer's server */
	list, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", ownPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	f := setLog(ownPort)
	defer f.Close()

	serverCertFile := fmt.Sprintf("%s.crt", p.name)
	serverKeyFile := fmt.Sprintf("%s.key", p.name)

	serverCert, err := credentials.NewServerTLSFromFile(serverCertFile, serverKeyFile)
	if err != nil {
		log.Fatalln("failed to create cert", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(serverCert))
	message.RegisterServiceServer(grpcServer, p)

	/* Starts the server */
	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("failed to serve %v", err)
		}
	}()

	/* Dials the other peers */
	for i := 0; i <= 3; i++ {
		port := hospitalId + int32(i)

		if port == ownPort {
			continue
		}

		clientName := portToName[port]
		clientCertFile := fmt.Sprintf("%s.crt", clientName)

		clientCert, err := credentials.NewClientTLSFromFile(clientCertFile, "")
		if err != nil {
			log.Fatalln("failed to create certificate", err)
		}

		fmt.Printf("%s | Trying to dial: %v\n", p.name, port)
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithTransportCredentials(clientCert), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect: %s", err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c

	}

	share1, share2, share3 := splitSecret(secret, R)

	if ownPort != hospitalId {
		p.shares = append(p.shares, int32(share3))
		log.Printf("%v | Secret: %v \n", portToName[ownPort], secret)
		log.Printf("%v | Secret into shares: %v %v %v \n", portToName[ownPort], share1, share2, share3)
	}
	/* Program loop */
	for {
		if ownPort != hospitalId {
			p.ShareData(share1, share2)
			time.Sleep(10 * time.Second)
			if len(p.shares) == 3 {
				p.SendToHospital(int(p.sumShares()))
				if p.name != "Hospital" {
					return
				}
			}
		}

	}
}

/* The sum of the peer's shares */
func (p *peer) sumShares() int32 {
	var sum int32 = 0
	for _, share := range p.shares {
		sum += share
	}
	return sum
}

/* Splits the secret into 3 random shares */
func splitSecret(secret, R int) (int, int, int) {
	rand.Seed(time.Now().UnixNano())

	share1 := rand.Intn(secret - 1)
	share2 := rand.Intn(secret - share1)
	share3 := secret - share1 - share2

	return share1, share2, share3
}

func (p *peer) SendToHospital(share int) {
	hospitalPort := int32(hospitalId)
	p.Send(hospitalPort, share)
}

/* gRPC method. Send message */
func (p *peer) Send(port int32, share int) {
	peerName := portToName[port]
	log.Printf("%s | Sending message to %s: %v", p.name, peerName, share)

	info := &message.Info{Id: p.id, SecretMessage: int32(share)}

	client, exists := p.clients[port]
	if !exists {
		log.Printf("%s | Client %s not found", p.name, peerName)
		return
	}

	// Sends the request to the selected peer
	_, err := client.Request(p.ctx, info)
	if err != nil {
		log.Printf("%s | Failed to send message to %s: %v", p.name, peerName, err)
	} else {
		log.Printf("%s | Successfully sent %v to %s", p.name, info.SecretMessage, peerName)
	}
}

/* Sends share1, share2 to different peers */
func (p *peer) ShareData(share1, share2 int) {
	log.Printf("%s | Sharing data chunks: %v and %v", p.name, share1, share2)

	var shareIndex int
	for id := range p.clients {
		if portToName[id] == "Hospital" {
			continue
		}
		var chunk int
		if shareIndex%3 == 0 {
			chunk = share1
		} else if shareIndex%3 == 1 {
			chunk = share2
		} else {
			break
		}
		p.Send(id, chunk)
		shareIndex++
	}
}

/* gRPC method. Receive message */
func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	peerName := portToName[req.Id]
	p.shares = append(p.shares, req.SecretMessage)
	log.Printf("%s | Received message from %s with share %v", p.name, peerName, req.SecretMessage)
	log.Printf("%s | Current shares: %v", p.name, p.shares)
	if p.name == "Hospital" && len(p.shares) == 3 {
		total := p.sumShares()
		log.Printf("%s | total sum %v", p.name, total)

	}
	return &message.Empty{}, nil
}

func setLog(port int32) *os.File {
	logID := portToName[port]
	filename := fmt.Sprintf("logs/%v-log.txt", logID)
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
	return f
}
