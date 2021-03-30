if [ `uname` = Linux ]; then
  export CC=i686-w64-mingw32-gcc
  export CXX=i686-w64-mingw32-g++
fi
GOOS=windows GOARCH=386 P=mingw32 LF="-static -Wl,--export-all-symbols -Wl,--whole-archive" LD="-Wl,--no-whole-archive -lgdi32 -lmsimg32 -lopengl32 -lwinmm -lws2_32 -lole32 -lpsapi -lmpr -lluajit -lstdc++" T="vxagent.exe" ./build.sh
