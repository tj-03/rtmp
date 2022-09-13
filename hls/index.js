const express = require('express')
const fs = require("fs")
const app = express()
const cors = require("cors")
var ffmpeg = require('fluent-ffmpeg')
const chokidar = require("chokidar")

// host, port and path to the RTMP stream
var host = 'localhost'
var port = '1935'
var path = '/twitch/twitch'

var corsOptions = {
    origin: '*',
  }

let dir = __dirname + "\\public\\videos\\";
fs.readdir(dir, (err, files) => {
    if (err) throw err;
  
    for (const file of files) {
      fs.unlink(dir + file, err => {
        if (err) throw err;
      });
    }
  });

  let virtualFiles = {}
  let watcher = chokidar.watch(dir);
  watcher.on('unlink', path => {
      if (path.endsWith(".m3u8") || path.endsWith(".ts")){
          delete virtualFiles[path]
      }
  })
  
  
  
  function update(path){
      fs.readFile(path, (err, data) => {
          if (err)
              throw err
          virtualFiles[path] = data
          data
      })
  }
  watcher.on('change', path => {
      if (path.endsWith(".m3u8") || path.endsWith(".ts")){
          update(path)
      }
  
  })
  
  watcher.on('add', path => {
      if (path.endsWith(".m3u8") || path.endsWith(".ts")){
          update(path)
      }
  
  })

ffmpeg('rtmp://'+host+':'+port+path, { timeout: 432000 }).addOptions([
    '-c:v copy',
    '-c:a aac',
    '-hls_time 2',
    '-hls_list_size 10',
    '-hls_flags delete_segments',
    '-hls_delete_threshold 12',
    '-start_number 1'
  ]).output('public/videos/index.m3u8').on("error",console.log).run()
// var child_process = require('child_process');

// child_process.exec(__dirname + '\\vid.bat', function(error, stdout, stderr) {
//     console.log(error);
// });


app.use(cors(corsOptions))
app.use(express.static('public'))
app.get('/live/:fname', (req, res) => {
    console.log("Endpoint hit")
    let fname = req.params.fname;
    console.log(fname)
    fname = __dirname + "\\public\\videos\\" + fname
    console.log("dir",fname)
    try{
        res.end(virtualFiles[fname], 'binary')
    }
    catch(err){
        console.log(err);
        res.status(500).send("err")
    }


})

app.listen(8000, () => {
  console.log(`Example app listening on port ${8000}`)
})

