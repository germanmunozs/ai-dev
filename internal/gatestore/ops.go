package gatestore

import (
	"io/fs"
	"os"
)

type ops struct {
	openRoot  func(string) (*os.Root, error)
	openDir   func(*os.Root, string) (*os.File, error)
	lstat     func(*os.Root, string) (fs.FileInfo, error)
	flock     func(*os.File) error
	readFile  func(*os.Root, string) ([]byte, error)
	openTmp   func(*os.Root, string, int, fs.FileMode) (*os.File, error)
	chmodTmp  func(*os.File, fs.FileMode) error
	writeTmp  func(*os.File, []byte) error
	syncTmp   func(*os.File) error
	closeTmp  func(*os.File) error
	rename    func(*os.Root, string, string) error
	remove    func(*os.Root, string) error
	syncDir   func(*os.File) error
	closeDir  func(*os.File) error
	afterStep func(string)
}
