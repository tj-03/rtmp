# RTMP Protocol Implementation

## About the project

This is an implementation of the RTMP protocol written in Go. As of now it can take in and process streams and connection from video software such as OBS Studio, VLC, and FFMPEG. AMF decoding and encoding was accomplished using the files from https://github.com/speps/go-amf (changes made). 

### Status

Currently the server works fine but there are potential multithreading issues that have not yet been worked on. 

### See also

* [go-amf module](https://github.com/speps/go-amf)

## How to use
Clone the repo and simply run 
```sh
go run ./src
```

