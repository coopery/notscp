package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/Unknwon/com"
)

func Listen(port int, hostKeyPath string) {
	config := &ssh.ServerConfig{
		PasswordCallback: AuthenticateClient,
	}

	privateKeyBytes, err := ioutil.ReadFile(hostKeyPath)
	if err != nil {
		fmt.Printf("Error reading host key %s: %s\n", hostKeyPath, err)
		return
	}

	privateKey, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		fmt.Printf("Error parsing host key file: %s\n", err)
		return
	}

	config.AddHostKey(privateKey)

	listen(config, port)
}

func listen(config *ssh.ServerConfig, port int) {
	// Start listening for incoming tcp connections
	listener, err := net.Listen("tcp", "127.0.0.1:"+com.ToStr(port))
	if err != nil {
		fmt.Printf("Error trying to listen to port %d\n", port)
		return
	}

	for {
		// Accept a tcp connection
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting tcp connection: %s\n", err)
			continue
		}

		// Perform ssh handshake over tcp conn
		sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
		if err != nil {
			fmt.Printf("Error setting up ssh connection: %s\n", err)
			continue
		}

		// TODO: look at what kind of requests actually come through here
		// Request channel must be serviced
		go ssh.DiscardRequests(reqs)
		go handleServerConn(sConn.Permissions.Extensions["user_id"], chans)
	}
}

func handleServerConn(keyID string, chans <-chan ssh.NewChannel) {
	fmt.Println("In handleServerConn()")

	// TODO: figure out why there can be multiple channels
	for newChan := range chans {
		fmt.Println("New channel creation request.")

		if chanType := newChan.ChannelType(); chanType != "session" {
			fmt.Println("Bad channel creation request for %s\n", chanType)
			newChan.Reject(ssh.UnknownChannelType, "Unknown channel type")
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			fmt.Println("Error accepting channel.")
			continue
		}

		fmt.Println("Accepted channel.")

		go serviceSshChannel(ch, reqs)
	}
}

func serviceSshChannel(ch ssh.Channel, in <-chan *ssh.Request) {
	req := <-in

	defer sendExitStatus(ch, req)
	defer ch.Close()

	fmt.Printf("Handling %s request.\n", req.Type)

	switch req.Type {
	case "shell":
		ch.Write([]byte("Nope no shell sorry.\n"))
		return

	case "exec":
		// Payload will be: garbage? scp flags directory
		cmdName := cleanCommand(string(req.Payload))
		fmt.Println("Command: " + cmdName)

		cmdFields := strings.Fields(cmdName)
		targetDir := cmdFields[len(cmdFields)-1]
		fmt.Println("to dir", targetDir)

		if cmdFields[0] != "scp" {
			fmt.Println("Illegal command given.")
			return
		}

		header := RecvNotScpHeader(ch)
		notscp_req := ParseNotScpHeader(header)

		perm := AskUserForPermission(
			fmt.Sprintf("Allow scp request? (y/n): %s", notscp_req))
		if !perm {
			fmt.Println("Permission denied for request.")
			sendExitStatus(ch, req)
			return
		}

		// Continue with scp
		cmdFields = cmdFields[1:]
		cmd := exec.Command("scp", cmdFields...)

		// pipe to send scp request (header + file) to local scp
		input, err := cmd.StdinPipe()
		if err != nil {
			fmt.Println("Error getting stdin pipe.")
			return
		}

		fmt.Println("Executing command.")

		err = cmd.Start()
		if err != nil {
			fmt.Println("Error starting command.")
			return
		}

		io.Copy(input, ch)
		fmt.Fprint(input, "\n")

		sendExitStatus(ch, req)

		err = cmd.Wait()
		if err != nil {
			// TODO: figure out why this happens every single time (seems bad)
			fmt.Println("Error waiting for command to return.")
			return
		}
	}
}

/**
 * Remove garbage from beginning of scp command
 */
func cleanCommand(cmd string) string {
	i := strings.Index(cmd, "scp")
	if i == -1 {
		return cmd
	}
	return cmd[i:]
}

func RecvNotScpHeader(ch ssh.Channel) string {
	header_buf := new(bytes.Buffer)
	b := make([]byte, 1)

	_, err := ch.Read(b)
	for err == nil {
		if string(b) == "\n" {
			break
		} else {
			header_buf.WriteByte(b[0])
		}
		_, err = ch.Read(b)
	}

	return header_buf.String()
}

func ParseNotScpHeader(header string) string {
	return header
}

func sendExitStatus(ch ssh.Channel, req *ssh.Request) {
	req.Reply(true, []byte{})
	ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
}

/**
 * Called when a client tries to initiate a connection with the server.
 */
func AuthenticateClient(conn ssh.ConnMetadata,
	password []byte) (*ssh.Permissions, error) {

	user := conn.User()

	fmt.Printf("Got request from user '%v' with password '%v'\n", user, string(password))

	question := fmt.Sprintf("Give %s permission to connect? (y/n)", user)
	userPerm := AskUserForPermission(question)

	if !userPerm {
		return nil, errors.New("Permission denied for connection.")
	}

	perms := ssh.Permissions{
		Extensions: map[string]string{"user_id": conn.User()},
	}

	return &perms, nil
}

func AskUserForPermission(question string) bool {
	for input := ""; input != "y"; {
		fmt.Println(question)

		fmt.Scanln(&input)

		if input == "n" {
			return false
		}
	}

	return true
}

func main() {
	port := flag.Int("p", 2222, "Port to listen for requests on.")

	// TODO: change to actual default keypath (see sshd -h)
	hostKey := flag.String("keypath",
		"/home/coopery/notscp/keys/ssh_host_rsa_key",
		"Path to host rsa key file")

	flag.Parse()

	fmt.Printf("Listening on port %d...\n", *port)

	Listen(*port, *hostKey)
}
