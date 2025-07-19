@echo off
REM =====================================
REM 开发运行脚本 - ics-dp
REM =====================================

cd src
go build -o ../ics-dp.exe main.go webshell.go csmp.go
cd ..