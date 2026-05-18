//go:generate protoc -I dfspb --gogofaster_out=plugins=grpc:dfspb dfspb/dfs.proto

package dfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/types/known/emptypb"

	"gopkg.qsoa.cloud/qdevrunner/dfs/dfspb"
)

const MaxChunk = 1024 * 1024

type Dfs struct {
	dfspb.UnimplementedDfsServer
	mu      sync.RWMutex
	buckets map[string]string
}

func New(buckets map[string]string) *Dfs {
	return &Dfs{
		buckets: buckets,
	}
}

func (s *Dfs) AddBucket(name, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buckets == nil {
		s.buckets = make(map[string]string)
	}
	s.buckets[name] = path
}

func (s *Dfs) RemoveBucket(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buckets, name)
}

func (s *Dfs) getBucket(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.buckets[name]
	return p, ok
}

func (s *Dfs) File(fs dfspb.Dfs_FileServer) error {
	var f *os.File

	for fs.Context().Err() == nil {
		req, err := fs.Recv()
		if err != nil {
			return err
		}

		switch msg := req.Msg.(type) {
		case *dfspb.FileReq_Open_:
			bucketFolder, exists := s.getBucket(msg.Open.Bucket)
			if !exists {
				if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Open_{Open: &dfspb.FileResp_Open{
					Error: &dfspb.FileResp_Error{
						Type: dfspb.FileResp_Error_NOT_EXIST,
						Msg:  fmt.Sprintf("unknown bucket '%s'", msg.Open.Bucket),
					},
				}}}); err != nil {
					return err
				}
			}
			f, err = os.OpenFile(filepath.Join(bucketFolder, msg.Open.Filename), int(msg.Open.Flag), 0600)
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Open_{Open: &dfspb.FileResp_Open{
				Error: toMsgErr(err),
			}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_Close_:
			//log.Printf("Close: %s", fName)
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Close_{Close: &dfspb.FileResp_Close{}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_Seek_:
			//log.Printf("Seek: %s", fName)
			n, err := f.Seek(msg.Seek.Offset, int(msg.Seek.Whence))
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Seek_{Seek: &dfspb.FileResp_Seek{
				N:     n,
				Error: toMsgErr(err),
			}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_Read_:
			//log.Printf("Read: %s", fName)
			n := msg.Read.N
			if n > MaxChunk {
				n = MaxChunk
			}
			buf := make([]byte, n)
			cnt, err := f.Read(buf)

			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Read_{Read: &dfspb.FileResp_Read{
				Data:  buf[:cnt],
				Error: toMsgErr(err),
			}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_Write_:
			//log.Printf("Write: %s", fName)
			n, err := f.Write(msg.Write.Data)
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Write_{Write: &dfspb.FileResp_Write{
				N:     int64(n),
				Error: toMsgErr(err),
			}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_ReadDir_:
			//log.Printf("Readdir: %s", fName)
			files, err := f.Readdir(int(msg.ReadDir.N))
			fileStats := make([]*dfspb.StatResp, len(files))
			for i, f := range files {
				fileStats[i] = &dfspb.StatResp{
					Name:    f.Name(),
					Size_:   f.Size(),
					ModTime: f.ModTime().Unix(),
					IsDir:   f.IsDir(),
				}
			}
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_ReadDir_{ReadDir: &dfspb.FileResp_ReadDir{
				Files: fileStats,
				Error: toMsgErr(err),
			}}}); err != nil {
				return err
			}

		case *dfspb.FileReq_Stat_:
			//log.Printf("Stat in File: %s", fName)
			stat, err := f.Stat()
			if err != nil {
				if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Stat_{Stat: &dfspb.FileResp_Stat{
					Error: toMsgErr(err),
				}}}); err != nil {
					return err
				}
			}
			if err := fs.Send(&dfspb.FileResp{Msg: &dfspb.FileResp_Stat_{Stat: &dfspb.FileResp_Stat{
				File: &dfspb.StatResp{
					Name:    stat.Name(),
					Size_:   stat.Size(),
					ModTime: stat.ModTime().Unix(),
					IsDir:   stat.IsDir(),
				},
			}}}); err != nil {
				return err
			}
		default:
			log.Fatalf("Unknow msg %T", msg)
		}
	}

	return nil
}

func (s *Dfs) MkDir(_ context.Context, req *dfspb.MkDirReq) (*emptypb.Empty, error) {
	bucketFolder, exists := s.getBucket(req.Bucket)
	if !exists {
		return nil, fmt.Errorf("unknown bucket '%s'", req.Bucket)
	}

	return &emptypb.Empty{}, os.MkdirAll(filepath.Join(bucketFolder, req.Filename), 0700)
}

func (s *Dfs) RemoveAll(_ context.Context, req *dfspb.RemoveAllReq) (*emptypb.Empty, error) {
	bucketFolder, exists := s.getBucket(req.Bucket)
	if !exists {
		return nil, fmt.Errorf("unknown bucket '%s'", req.Bucket)
	}

	return &emptypb.Empty{}, os.RemoveAll(filepath.Join(bucketFolder, req.Filename))
}

func (s *Dfs) Rename(_ context.Context, req *dfspb.RenameReq) (*emptypb.Empty, error) {
	bucketFolder, exists := s.getBucket(req.Bucket)
	if !exists {
		return nil, fmt.Errorf("unknown bucket '%s'", req.Bucket)
	}

	return &emptypb.Empty{}, os.Rename(filepath.Join(bucketFolder, req.OldName), filepath.Join(bucketFolder, req.NewName))
}

func (s *Dfs) Stat(_ context.Context, req *dfspb.StatReq) (*dfspb.StatResp, error) {
	bucketFolder, exists := s.getBucket(req.Bucket)
	if !exists {
		return nil, fmt.Errorf("unknown bucket '%s'", req.Bucket)
	}

	info, err := os.Stat(filepath.Join(bucketFolder, req.Filename))
	if err != nil {
		return nil, err
	}

	return &dfspb.StatResp{
		Name:    filepath.Clean(req.Filename),
		Size_:   info.Size(),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}, nil
}

func (s *Dfs) Read(_ context.Context, req *dfspb.StatReq) (*dfspb.StatResp, error) {
	bucketFolder, exists := s.getBucket(req.Bucket)
	if !exists {
		return nil, fmt.Errorf("unknown bucket '%s'", req.Bucket)
	}

	info, err := os.Stat(filepath.Join(bucketFolder, req.Filename))
	if err != nil {
		return nil, err
	}

	return &dfspb.StatResp{
		Name:    filepath.Clean(req.Filename),
		Size_:   info.Size(),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}, nil
}

func toMsgErr(err error) *dfspb.FileResp_Error {
	if err == nil {
		return nil
	}

	errType := dfspb.FileResp_Error_OTHER

	if os.IsNotExist(err) {
		errType = dfspb.FileResp_Error_NOT_EXIST
	} else if os.IsExist(err) {
		errType = dfspb.FileResp_Error_EXIST
	} else if errors.Is(err, io.EOF) {
		errType = dfspb.FileResp_Error_EOF
	} else if errors.Is(err, io.ErrUnexpectedEOF) {
		errType = dfspb.FileResp_Error_UNEXPECTED_EOF
	}

	return &dfspb.FileResp_Error{
		Type: errType,
		Msg:  err.Error(),
	}
}
