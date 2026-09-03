#!/bin/bash

# Copyright 2026 PANTHEON.tech s.r.o.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

PANDOC_LATEX_IMAGE="${PANDOC_LATEX_IMAGE}"

SOURCE_DIR="${SOURCE_DIR}"
INPUT_FILE="${INPUT_FILE}"
OUTPUT_FILE="${OUTPUT_FILE}"

if [ "$TABLE_OF_CONTENTS" == "true" ]; then
    TOC_OPTION="--table-of-contents"
else
    TOC_OPTION=""
fi

function generate {
    # arguments: input file, output file, log file (set it to empty for no log file)

    if [ "$3" = "" ]; then
        LOG_OPTION=""
    else
        LOG_OPTION="--log=$3"
    fi

    docker run --rm --volume "$TEMP_DIR:/data" --user "`id -u`":"`id -g`" "$PANDOC_LATEX_IMAGE" \
        "$1" \
        -o "$2" \
        --from markdown \
        --variable geometry:margin=2.5cm \
        --variable colorlinks \
        --variable fontsize=12pt \
	    --highlight-style=zenburn \
        $TOC_OPTION \
        $LOG_OPTION
}

TEMP_DIR=$(mktemp -d)
cd "$SOURCE_DIR"
cp -R . "$TEMP_DIR"

# ---- Generate temporary pdf file and check for too long lines

# Overfull warnings are not emitted for code blocks with syntax highlighting (e.g. "```bash")
cp "$TEMP_DIR/$INPUT_FILE" "$TEMP_DIR/input-stripped.md"
sed -i -E 's/```[a-z]+/```/g' "$TEMP_DIR/input-stripped.md"

generate "input-stripped.md" "output-temp.pdf" "log.txt"
sed -i -E 's/\\n/\n/g' "$TEMP_DIR/log.txt"

grep "Overfull" -A 2 "$TEMP_DIR/log.txt" || true
# The outputted line numbers are for the intermediate .tex file, not for the original .md file.
# You can use text search to find the offending places in the original .md file.

# ---- Now generate final pdf file

generate "$INPUT_FILE" "$OUTPUT_FILE" ""

cp "$TEMP_DIR/$OUTPUT_FILE" "$SOURCE_DIR/$OUTPUT_FILE"

rm -rf "$TEMP_DIR"
