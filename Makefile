# Updater Makefile
#
PROJECT	:= container-updater
GO			:= /usr/lib/go-1.10/bin/go
MKDIR		:= mkdir -p
RM			:= rm -rf
ECHO		:= echo
#GOPATH	:= $(shell pwd)
GOPATH  := ${HOME}/go
BUILD		:= build/$(PROJECT)
PROJECTPATH := src/github.com/zex/container-update

.PHONY: clean updated build all tests

all: build updaterd

build:
	$(MKDIR) $(BUILD)

dep:
	$(MKDIR) $(GOPATH)/src
	@ln -s /usr/share/gocode/src/* $(GOPATH)/src

updated: $(GOPATH)/$(PROJECTPATH)/apps/updated.go
	$(ECHO) "creating $@"
	GOPATH=$(GOPATH) go build -o $(BUILD)/updaterd $<

clean:
	$(RM) $(BUILD)
