package main

import (
	"cloud.google.com/go/kms/apiv1"
	"context"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	"os"
	"time"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of decrypting with gcloud kms")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "kms")
	driver.RPCHandlers["decrypt"] = decrypt
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func decrypt(from string, rpcID string, payload []byte) {
	name, ok := driver.GetOptionStr("data.keyname")
	if !ok {
		driver.RPCError(from, rpcID, "key not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	req := &kmspb.DecryptRequest{
		Name:       name,
		Ciphertext: payload,
	}
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		driver.RPCError(from, rpcID, "failed to create kms client")
	}
	resp, err := client.Decrypt(ctx, req)
	if err != nil {
		driver.RPCError(from, rpcID, "failed to decrypt")
	}
	driver.RPCReturnRaw(from, rpcID, resp.Plaintext)
}
