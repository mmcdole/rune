#!/bin/bash

# Combine all Go and Lua files into allcode.txt

OUTPUT="allcode.txt"

# Clear/create output file
> "$OUTPUT"

# Find and concatenate all .go and .lua files
find . -type f \( -name "*.go" -o -name "*.lua" \) | sort | while read -r file; do
    echo "========== $file ==========" >> "$OUTPUT"
    cat "$file" >> "$OUTPUT"
    echo "" >> "$OUTPUT"
done

echo "Combined code written to $OUTPUT"
