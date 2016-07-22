package main

/*
#cgo CFLAGS: -I . -I/usr/include/glib-2.0 -I/usr/include/glib-2.0 -I/usr/lib/x86_64-linux-gnu/glib-2.0/include -pthread -I/usr/include/gtk-2.0 -I/usr/lib/x86_64-linux-gnu/gtk-2.0/include -I/usr/include/gio-unix-2.0/ -I/usr/include/cairo -I/usr/include/pango-1.0 -I/usr/include/atk-1.0 -I/usr/include/cairo -I/usr/include/pixman-1 -I/usr/include/libpng12 -I/usr/include/gdk-pixbuf-2.0 -I/usr/include/libpng12 -I/usr/include/pango-1.0 -I/usr/include/harfbuzz -I/usr/include/pango-1.0 -I/usr/include/glib-2.0 -I/usr/lib/x86_64-linux-gnu/glib-2.0/include -I/usr/include/freetype2
#cgo LDFLAGS: -L . -lnotify -lgdk_pixbuf-2.0 -lgio-2.0 -lgobject-2.0 -lglib-2.0

#include <libnotify/notify.h>

void c_callback(NotifyNotification *notification, char *action, gpointer user_data);
*/
import "C"
import notify "github.com/mqu/go-notify"
import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

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

func handleServerConn(user string, chans <-chan ssh.NewChannel) {
	fmt.Println("In handleServerConn()")
	fmt.Println("Connection with user: ", user)

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

		go serviceSshChannel(ch, reqs, user)
	}
}

func serviceSshChannel(ch ssh.Channel, in <-chan *ssh.Request, user string) {
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
		expanded, err := tildeExpansion(cmdFields[3], user)
		cmdFields[3] = expanded
		cmd := exec.Command(cmdFields[0], cmdFields[1:]...)
		fmt.Println(cmd.Path, cmd.Args)

		// Send scp stderr to our stderr
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout

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

		//sendExitStatus(ch, req)

		err = cmd.Wait()
		if err != nil {
			// TODO: figure out why this happens every single time (seems bad)
			// ** Need to send output of scp fork to other side, respond to
			// messages, like E with a \x00
			fmt.Println("Error waiting for command to return.")
			fmt.Println(err)
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

/**
 * Expand tilde in path to home directory if possible
 * TODO: figure out how to run "sh -c 'scp ...'" instead
 * of "scp ..." so we can let the shell do this
 */
func tildeExpansion(path, username string) (string, error) {
	if string(path[0]) != "~" {
		return path, nil
	}

	user_account, err := user.Lookup(username)
	if err != nil {
		fmt.Println(err)
		return path, err
	}

	home_dir := user_account.HomeDir

	expanded := strings.Replace(path, "~", home_dir, 1)

	fmt.Println("home directory:", home_dir)
	fmt.Println("home directory:", expanded)

	return expanded, nil
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

//export callOnMeGo
//func callOnMeGo(notification *C.NotifyNotification, action string, user_data unsafe.Pointer) {
//	fmt.Println("in go callback")
//}

func sendNotification(title, description string) {
	notification := notify.NotificationNew(title, description, "")

	if notification == nil {
		fmt.Fprintf(os.Stderr, "Unable to create a notification.\n")
		return
	}

	notification.SetTimeout(10000)

	// lol place attempts at notification callback here
	//notification.AddAction("action", "label", (C.NotifyActionCallback)(unsafe.Pointer(C.callOnMeGo_cgo)), nil)
	//C.bridge((*C.struct__NotifyNotification)(unsafe.Pointer(hello)));
	//C.notify_notification_add_action((*C.struct__NotifyNotification)(unsafe.Pointer(hello)), C.CString("action"), C.CString("label"), (C.NotifyActionCallback)(unsafe.Pointer(C.callOnMeGo_cgo)), nil, nil)

	notification.Show()
	time.Sleep(1000000)
	notification.Close()
}

func main() {
	port := flag.Int("p", 2222, "Port to listen for requests on.")

	// TODO: change to actual default keypath (see sshd -h)
	hostKey := flag.String("keypath",
		"/home/coopery/notscp/keys/ssh_host_rsa_key",
		"Path to host rsa key file")

	flag.Parse()

	notify.Init("notscp")

	fmt.Printf("Listening on port %d...\n", *port)

	Listen(*port, *hostKey)

	notify.UnInit()
}
