[ "$PACKAGE_VER" ] || PACKAGE_VER=$(git describe --tags `git rev-list --tags --max-count=1`)
[ "$PACKAGE_REV" ] || PACKAGE_REV=$(git rev-parse --short HEAD)

export BASE_PREFIX=../vxcommon/lib
if [ ! -d "$BASE_PREFIX" ]; then
    export VXCOMMON=`go list -m github.com/vxcontrol/vxcommon | sed 's/ /@/g'`
    export BASE_PREFIX=`go env | grep GOPATH | cut -d'"' -f2 | xargs -I {} echo "{}/pkg/mod/$VXCOMMON/lib"`
fi
# go get
# for debugging -gcflags="all=-N -l" 
go build -ldflags "-X main.PackageVer=$PACKAGE_VER -X main.PackageRev=$PACKAGE_REV -L $BASE_PREFIX/$P -extldflags '$LF $BASE_PREFIX/$P/libluab.a $LD'" -o build/$T main.go
