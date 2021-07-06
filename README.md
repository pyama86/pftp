[![Build Status](https://app.travis-ci.com/heat1024/pftp.svg?branch=master-heat1024)](https://app.travis-ci.com/github/heat1024/pftp)

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

	if err := ftpServer.Start(); err != nil {
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

## Require
- Go 1.15 or later

# author
- @pyama86
- @heat1024
