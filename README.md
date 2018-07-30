[![Build Status](https://travis-ci.org/pyama86/pftp.svg?branch=master)](https://travis-ci.org/pyama86/pftp)

# pftp
plaggable ftp proxy server

# example
```go
func main() {
	confFile := "./example.toml"

	ftpServer, err := pftp.NewFtpServer(confFile)
	if err != nil {
		logrus.Fatal(err)
	}
	go signalHandler()

	if err := ftpServer.ListenAndServe(); err != nil {
		logrus.Fatal(err)
	}
}
```

## middleware
In pftp, you can hook into the ftp command and execute arbitrary processing.

### USER command example
An example of changing the connection destination according to the user name.
```go
func main() {
...
	ftpServer.Use("user", User)
...
}

func User(c *pftp.Context, param string) error {
        if param == "foo" {
	    c.RemoteAddr = "127.0.0.1:10021"
        } else if param == "bar" {
	    c.RemoteAddr = "127.0.0.1:20021"
        }
	return nil
}
```

# author
@pyama86
