package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/crypto/ssh"

	"github.com/Unknwon/com"
)

func Listen(port int) {
	fmt.Println("In Listen()")

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			fmt.Printf("Got request from user '%v' with password '%v'\n",
				conn.User(), string(password))

			perms := ssh.Permissions {
				Extensions: map[string]string{"user_id": conn.User()},
			}

			return &perms, nil
		},
	}

	keyPath := "/home/coopery/notscp/keys/ssh_host_rsa_key"

	privateBytes, err := ioutil.ReadFile(keyPath)
	if err != nil { return }
	privateKey, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil { return }

	config.AddHostKey(privateKey)

	listen(config, port)
}

func listen(config *ssh.ServerConfig, port int) {
	fmt.Println("In listen()")
	listener, err := net.Listen("tcp", "127.0.0.1:" + com.ToStr(port))
	if err != nil { return }

	for {
		conn, err := listener.Accept()
		if err != nil { return }

		sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
		if err != nil { return }

		go ssh.DiscardRequests(reqs)
		go handleServerConn(sConn.Permissions.Extensions["user_id"], chans)
	}
}

func handleServerConn(keyID string, chans <-chan ssh.NewChannel) {
	fmt.Println("In handleServerConn()");

	for newChan := range chans {
		fmt.Println("New channel creation request.");

		if chanType := newChan.ChannelType(); chanType != "session" {
			fmt.Println("Bad channel creation request for %s\n", chanType);
			newChan.Reject(ssh.UnknownChannelType, "Unknown channel type");
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			fmt.Println("Error accepting channel.")
			return
		}

		fmt.Println("Accepted channel.");

		go func(in <-chan *ssh.Request) {
			fmt.Println("In func")
			defer ch.Close()
			for req := range in {
				fmt.Printf("Handling %v request.\n", req.Type)

				switch req.Type {
				case "shell":
					ch.Write([]byte("Nope no shell sorry.\n"))
					continue

				case "exec":
					// Payload will be: garbage? scp flags directory
					cmdName := cleanCommand(string(req.Payload))
					fmt.Println("Command: " + cmdName)

					cmdFields := strings.Fields(cmdName)
					targetDir := cmdFields[len(cmdFields) - 1]
					fmt.Println("to dir", targetDir)

					if cmdFields[0] != "scp" {
						fmt.Println("Illegal command given.")
						return
					}

					header := RecvNotScpHeader(ch)

					notscp_req := ParseNotScpHeader(header)

					perm := AskUserForPermission(notscp_req)

					if !perm {
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

					// say we good
					req.Reply(true, []byte{})
					ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})

					err = cmd.Wait()
					if err != nil {
						fmt.Println("Error waiting for command to return.")
						return
					}
				}
			}
		}(reqs)
	}
}

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

func AskUserForPermission(request string) bool {
	fmt.Printf("Received request: %s\n", request)

	for input := ""; input != "y"; {
		fmt.Println("Allow scp request?")

		fmt.Scanln(&input)

		if input == "n" {
			fmt.Println("Rejecting request")
			return false
		}
	}

	fmt.Println("Giving permission for request")

	return true
}

func main() {
	Listen(2222)
}
