package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

const (
	exitCodeOk    int = 0
	exitCodeError int = 1
)

var (
	wg sync.WaitGroup

	iamGroup    = ""
	sshUserName = ""
)

func main() {
	sess, _ := session.NewSession()
	svc := iam.New(sess)

	// check for valid user name
	if sshUserName != "" && (len(os.Args) < 2 || os.Args[1] != sshUserName) {
		os.Exit(exitCodeOk)
	}

	// Handle SIGPIPE
	//
	// When sshd identifies a key in the stdout of this command, it closes
	// the pipe causing a series of EPIPE errors before a SIGPIPE is emitted
	// on this scripts pid. If the script exits with the standard 13 code, sshd
	// will disregard any matched keys. We catch the signal here and exit 0 to
	// fix that problem.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	go func() {
		_ = <-c
		os.Exit(exitCodeOk)
	}()

	users, err := users(svc, iamGroup)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitCodeError)
	}

	for _, u := range users {
		go func(userName *string) {
			wg.Add(1)
			params := &iam.ListSSHPublicKeysInput{
				UserName: userName,
			}
			if resp, err := svc.ListSSHPublicKeys(params); err == nil {
				for _, k := range resp.SSHPublicKeys {
					if *k.Status != "Active" {
						continue
					}
					params := &iam.GetSSHPublicKeyInput{
						Encoding:       aws.String("SSH"),
						SSHPublicKeyId: k.SSHPublicKeyId,
						UserName:       userName,
					}
					resp, _ := svc.GetSSHPublicKey(params)
					fmt.Printf("# %s\n", *userName)
					fmt.Println(*resp.SSHPublicKey.SSHPublicKeyBody)
				}
			}
			wg.Done()
		}(u.UserName)
	}
	wg.Wait()
}

// get all IAM users, or just those that are part of the defined group
func users(svc *iam.IAM, iamGroup string) ([]*iam.User, error) {
	if iamGroup != "" {
		params := &iam.GetGroupInput{
			GroupName: aws.String(iamGroup),
		}
		resp, err := svc.GetGroup(params)
		return resp.Users, err
	}
	params := &iam.ListUsersInput{
		MaxItems: aws.Int64(100),
	}
	resp, err := svc.ListUsers(params)
	return resp.Users, err
}
