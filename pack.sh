#!/bin/bash
set -e
zip -r easyms.zip . -x "*/.git/*"
echo "打包完成: easyms.zip"
