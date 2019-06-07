#!/bin/bash

gen=tests/gen_mani.go

export MAJOR_VERSION=3
export ENABLE_UPDATED=1
export ENABLE_DB=1
export ENABLE_APP=1
export ASSET_HOST=https://asset.duck.co
export ASSET_USER=duck
export ASSET_PASS=yellow78
export SUB_URI=https://subscribe.duck.co
export SUB_USER=duck2
export SUB_PASS=green44
export ID=`uuidgen`

update=`go run $gen -encode update`
echo "Encoded UpdateManifest"
echo "$update"
echo "Decoded UpdateManifest"
go run $gen -decode update -data "$update"

sub=`go run $gen -encode sub`
echo "Encoded SubManifest"
echo "$sub"
echo "Decoded SubManifest"
go run $gen -decode sub -data "$sub"

asset=`go run $gen -encode asset`
echo "Encoded AssetManifest"
echo "$asset"
echo "Decoded AssetManifest"
go run $gen -decode asset -data "$asset"
