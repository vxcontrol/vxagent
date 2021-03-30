GOOS=linux GOARCH=amd64 P=linux64 LF="-Wl,--whole-archive" LD="-Wl,--no-whole-archive -pthread -lluajit -lm -ldl -lstdc++" T="vxagent" ./build.sh
