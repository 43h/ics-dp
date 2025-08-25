@echo off
REM =====================================
REM 开发运行脚本 - ics-dp
REM =====================================

REM 编译 Linux 版本
set GOOS=linux
set GOARCH=amd64
cd src
go build -o ../ics-dp-linux main.go webshell.go csmp.go vncAddress.go vnc.go device.go
cd ..
set GOOS=
set GOARCH=