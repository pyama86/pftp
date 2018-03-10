package pftp

import (
	"io/ioutil"
	"os"

	"github.com/fclairamb/ftpserver/server"
)

type ClientDriver struct {
	BaseDir string
}

func (driver *ClientDriver) ChangeDirectory(cc server.ClientContext, directory string) error {
	_, err := os.Stat(driver.BaseDir + directory)
	return err
}

func (driver *ClientDriver) MakeDirectory(cc server.ClientContext, directory string) error {
	return os.Mkdir(driver.BaseDir+directory, 0777)
}

func (driver *ClientDriver) ListFiles(cc server.ClientContext) ([]os.FileInfo, error) {
	path := driver.BaseDir + cc.Path()
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	return files, err
}

func (driver *ClientDriver) OpenFile(cc server.ClientContext, path string, flag int) (server.FileStream, error) {
	path = driver.BaseDir + path
	if (flag & os.O_WRONLY) != 0 {
		flag |= os.O_CREATE
		if (flag & os.O_APPEND) == 0 {
			os.Remove(path)
		}
	}
	return os.OpenFile(path, flag, 0666)
}

func (driver *ClientDriver) GetFileInfo(cc server.ClientContext, path string) (os.FileInfo, error) {
	path = driver.BaseDir + path
	return os.Stat(path)
}

func (driver *ClientDriver) CanAllocate(cc server.ClientContext, size int) (bool, error) {
	return true, nil
}

func (driver *ClientDriver) ChmodFile(cc server.ClientContext, path string, mode os.FileMode) error {
	path = driver.BaseDir + path
	return os.Chmod(path, mode)
}

func (driver *ClientDriver) DeleteFile(cc server.ClientContext, path string) error {
	path = driver.BaseDir + path
	return os.Remove(path)
}

func (driver *ClientDriver) RenameFile(cc server.ClientContext, from, to string) error {
	from = driver.BaseDir + from
	to = driver.BaseDir + to
	return os.Rename(from, to)
}
