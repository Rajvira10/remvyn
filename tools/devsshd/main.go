// Command devsshd runs Remvyn's in-process test SSH server for manual testing:
// password auth, a tiny shell, SFTP and port forwarding.
//
//	go run ./tools/devsshd -addr 127.0.0.1:2223 -user demo -password demo
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"golang.org/x/crypto/ssh"

	"remvyn/backend/testutil"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2223", "listen address")
	user := flag.String("user", "demo", "username")
	password := flag.String("password", "demo", "password")
	flag.Parse()

	srv, err := testutil.NewServerOn(*addr, *user, *password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("devsshd listening on %s (user %q)\nhost key %s\n", srv.Addr, *user, ssh.FingerprintSHA256(srv.HostKey.PublicKey()))
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	srv.Close()
}
