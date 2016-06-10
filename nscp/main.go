package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func getKeyFile(keypath string) (ssh.Signer, error) {
	user, err := user.Current()
	if err != nil {
		return nil, err
	}

	file := user.HomeDir + keypath
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	pubkey, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}

	return pubkey, nil
}

type ConnConfig struct {
	User		string
	Server		string
	Key			string
	Port		string
	Password	string
}

func (ssh_conf *ConnConfig) connect() (*ssh.Session, error) {
	auths := []ssh.AuthMethod{}

	if ssh_conf.Password != "" {
		auths = append(auths, ssh.Password(ssh_conf.Password))
	}

	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
		defer sshAgent.Close()
	}

	if pubkey, err := getKeyFile(ssh_conf.Key); err == nil {
		auths = append(auths, ssh.PublicKeys(pubkey))
	}

	config := &ssh.ClientConfig{
		User: ssh_conf.User,
		Auth: auths,
	}

	client, err := ssh.Dial("tcp", ssh_conf.Server + ":" + ssh_conf.Port, config)
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (ssh_conf *ConnConfig) Scp(srcFile string, dstDir string) error {
	session, err := ssh_conf.connect()
	if err != nil { return err }
	defer session.Close()

	targetFile := filepath.Base(srcFile)

	src, err := os.Open(srcFile)
	if err != nil { return err }

	srcStat, err := src.Stat()
	if err != nil { return err }

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		// Create notscp header [F/D filename size F/D filename size ...\n]
		send_buf := new(bytes.Buffer)
		file_type := "F"
		if srcStat.IsDir() {
			file_type = "D"
		}
		fmt.Fprintln(send_buf, file_type, srcStat.Name(), srcStat.Size())

		// Send notscp header size, then header
//		fmt.Fprint(w, send_buf.Len(), send_buf)
		fmt.Fprint(w, send_buf)

		// Send scp header [type + mode, length, filename]
		fmt.Fprintln(w, "C0644", srcStat.Size(), targetFile)

		// Send file data and sentinel
		io.Copy(w, src)
		fmt.Fprint(w, "\x00")

		fmt.Println("Finished copying to remote")
	}()

	err = session.Run(fmt.Sprintf("scp -t %s", dstDir))
	if err != nil {
		fmt.Println("Err", err)
		return err
	}

	fmt.Println("Leaving Scp()")
	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Need source and destination")
		return
	}

	src, dst := os.Args[1], os.Args[2]

	fmt.Println("Copying from %s to %s", src, dst)

	config := ConnConfig{
		User: "coopery",
		Server: "127.0.0.1",
		Key: "notscp/keys/ssh_host_rsa_key",
		Port: "2222",
		Password: "hkels",
	}

	config.Scp(src, dst)
}