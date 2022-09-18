# RTMP Protocol Implementation

## About the project

This is an implementation of RTMP written in Go. As of now it can take in and process streams and connection from video software such as OBS Studio, VLC, and FFMPEG.

There is also a simple Javascript HLS server to watch the stream in a browser.

### Status

Currently the server works fine but there are potential concurrency issues that have not yet been worked on. 

## How to use
Clone the repo and simply run 
```sh
go run ./src
```
If you want to run the HLS server make sure you have FFMPEG installed and in your local PATH.
