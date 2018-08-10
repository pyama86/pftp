package main

import (
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jlaffaye/ftp"
	"github.com/marcobeierer/ftps"
	"golang.org/x/sync/errgroup"
)

var (
	integration = flag.Bool("integration", false, "run integration tests")
)

const testCount = 5

type userInfo struct {
	ID   string
	Pass string
}

type testSet struct {
	User userInfo
	Dir  string
}

var testset = []testSet{
	testSet{userInfo{"vsuser", "vsuser"}, "misc/test/data/vsuser"},
}

func localConnect(port int, t *testing.T) *ftp.ServerConn {
	client, err := ftp.Connect(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatalf("integration.localConnect() error = %v, wantErr %v", err, nil)
	}
	return client
}

func loggedin(port int, t *testing.T, user userInfo) *ftp.ServerConn {
	client := localConnect(port, t)

	err := client.Login(user.ID, user.Pass)
	if err != nil {
		t.Fatalf("integration.loggedin() error = %v, wantErr %v", err, nil)
	}
	return client
}

func TestMain(m *testing.M) {
	flag.Parse()
	result := m.Run()
	os.Exit(result)
}

func TestConnect(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := localConnect(2121, t)
	defer client.Quit()
}

func TestLogin(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := localConnect(2121, t)
	defer client.Quit()

	var err error
	// If Login failed with vsftpd & proftpd user, Return Error
	for i := 0; i < len(testset); i++ {
		err = client.Login(testset[i].User.ID, testset[i].User.Pass)
		if err != nil {
			t.Errorf("integration.TestLogin() error = %v, wantErr %v", err, nil)
		}
	}

	err = client.Login("hoge", "moge")
	if err != nil {
		if err.Error() != "530 Login incorrect." {
			t.Errorf("integration.TestLogin() error = %v, wantErr %v", err, nil)
		}
	}
}

func TestAuth(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := new(ftps.FTPS)
	defer client.Quit()
	client.Debug = true
	client.TLSConfig.InsecureSkipVerify = true

	err := client.Connect("localhost", 2121)
	if err != nil {
		t.Errorf("integration.TestAuth() error = %v, wantErr %v", err, nil)
	}

	// If Login failed with vsftpd & proftpd user, Return Error
	for i := 0; i < len(testset); i++ {
		err = client.Login(testset[i].User.ID, testset[i].User.Pass)
		if err == nil {
			if err.Error() != "530 Login incorrect." {
				t.Errorf("integration.TestAuth() wantErr %v", errors.New("550 Permission denied."))
			}
		}
	}
}

func removeDirFiles(t *testing.T, dir string) {
	for i := 0; i < len(testset); i++ {
		f := path.Join(testset[i].Dir, dir)
		filepath.Walk(f,
			func(fpath string, info os.FileInfo, err error) error {
				rel, err := filepath.Rel(f, fpath)
				if err != nil {
					t.Fatal(err)
				}
				if rel == `.` || rel == `..` {
					return nil
				}
				out, err := exec.Command("rm", "-f", fpath).CombinedOutput()
				if err != nil {
					t.Fatal(string(out))
				}
				return err
			})
	}
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func makeRandomFiles(t *testing.T) {
	eg := errgroup.Group{}
	for i := 0; i < testCount; i++ {
		testIndex := i % len(testset)
		num := i

		eg.Go(func() error {
			f := fmt.Sprintf("%s/%d", testset[testIndex].Dir, num)
			if !fileExists(f) {
				// make 500MB files
				out, err := exec.Command("dd", "if=/dev/urandom", fmt.Sprintf("of=%s", f), "bs=1024", "count=50000").CombinedOutput()
				if err != nil {
					return errors.New(string(out))
				}
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestDownload(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	eg := errgroup.Group{}

	makeRandomFiles(t)

	c := make(chan bool, testCount+1)
	for i := 0; i < testCount; i++ {
		c <- true
		num := i
		testIndex := i % len(testset)

		eg.Go(func() error {
			defer func() { <-c }()
			a := md5.New()
			b := md5.New()

			client := loggedin(2121, t, testset[testIndex].User)
			defer client.Quit()

			r, err := client.Retr(fmt.Sprintf("%d", num))
			if err != nil {
				return err
			}

			_, err = io.Copy(a, r)
			if err != nil {
				return err
			}

			f, err := os.Open(fmt.Sprintf("%s/%d", testset[testIndex].Dir, num))
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(b, f)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(a.Sum(nil), b.Sum(nil)) {
				errors.New(fmt.Sprintf("download file check sum error: %d", num))
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestUpload(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	eg := errgroup.Group{}

	removeDirFiles(t, "stor")

	c := make(chan bool, testCount+1)
	for i := 0; i < testCount; i++ {
		c <- true
		num := i
		testIndex := i % len(testset)

		eg.Go(func() error {
			defer func() { <-c }()
			a := md5.New()
			b := md5.New()

			client := loggedin(2121, t, testset[testIndex].User)
			defer client.Quit()

			f, err := os.Open(fmt.Sprintf("%s/%d", testset[testIndex].Dir, num))
			if err != nil {
				return err
			}
			defer f.Close()

			if err := os.MkdirAll(fmt.Sprintf("%s/stor", testset[testIndex].Dir), 0777); err != nil {
				return err
			}

			err = client.Stor(fmt.Sprintf("stor/%d", num), f)
			if err != nil {
				return err
			}

			s, err := os.Open(fmt.Sprintf("%s/stor/%d", testset[testIndex].Dir, num))
			if err != nil {
				return err
			}
			defer s.Close()

			_, err = io.Copy(a, s)
			if err != nil {
				return err
			}

			_, err = io.Copy(b, f)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(a.Sum(nil), b.Sum(nil)) {
				errors.New(fmt.Sprintf("upload file check sum error: %d", num))
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}
