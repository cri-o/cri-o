#!/bin/bash
go_files=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./_output/*" -not -path "./test/mocks/*" -not -path "./utils/fifo/*")
echo "$go_files" | xargs sed -i 's|log\.\(.*\)f.ctx\, \"\([a-z]\)|log.\1f\(ctx\, \"\u\2|g'
echo "$go_files" | xargs sed -i 's|logrus.\(.*\)f.\"\([a-z]\)|logrus.\1f\(\"\u\2|g'
