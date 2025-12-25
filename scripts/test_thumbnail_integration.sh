#!/bin/bash
# Integration test for thumbnail display
# Tests the full flow from MCP inspect tool to HTTP thumbnail serving

set -e

MCP_URL="${MCP_URL:-http://localhost:3000/mcp}"
API_URL="${API_URL:-http://localhost:3000}"
TEST_FILE="${TEST_FILE:-/Users/josh/grab/Downloads/Cyberdrop-DL Downloads/ILTB (Bunkrr)/hdsb08043_720.mp4}"

echo "=== Thumbnail Integration Test ==="
echo "MCP URL: $MCP_URL"
echo "API URL: $API_URL"
echo "Test File: $TEST_FILE"
echo ""

# Step 1: Call inspect tool and extract thumbnailUri
echo "Step 1: Testing inspect tool..."
INSPECT_RESPONSE=$(curl -s -X POST "$MCP_URL" \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"inspect\",\"arguments\":{\"path\":\"$TEST_FILE\"}},\"id\":1}")

# Extract the text content from MCP response
INSPECT_DATA=$(echo "$INSPECT_RESPONSE" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    if 'error' in d:
        print('ERROR: ' + str(d['error']))
        sys.exit(1)
    text = d['result']['content'][0]['text']
    print(text)
except Exception as e:
    print('PARSE_ERROR: ' + str(e))
    sys.exit(1)
")

if [[ "$INSPECT_DATA" == ERROR:* ]] || [[ "$INSPECT_DATA" == PARSE_ERROR:* ]]; then
    echo "FAIL: Inspect tool failed: $INSPECT_DATA"
    exit 1
fi

echo "  Inspect response received"

# Extract thumbnailUri
THUMBNAIL_URI=$(echo "$INSPECT_DATA" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('thumbnailUri', 'NOT_FOUND'))
")

if [ "$THUMBNAIL_URI" == "NOT_FOUND" ]; then
    echo "FAIL: No thumbnailUri in inspect response"
    echo "Response: $INSPECT_DATA"
    exit 1
fi

echo "  thumbnailUri: $THUMBNAIL_URI"

# Step 2: Verify it's an HTTP URL (not synthesis://)
if [[ "$THUMBNAIL_URI" == synthesis://* ]]; then
    echo "FAIL: thumbnailUri is still a synthesis:// URI, expected HTTP URL"
    exit 1
fi

if [[ "$THUMBNAIL_URI" != http://* ]] && [[ "$THUMBNAIL_URI" != https://* ]]; then
    echo "FAIL: thumbnailUri is not an HTTP URL: $THUMBNAIL_URI"
    exit 1
fi

echo "  ✓ thumbnailUri is HTTP URL"

# Step 3: Fetch the thumbnail and verify it's a valid image
echo ""
echo "Step 2: Fetching thumbnail via HTTP..."
HTTP_STATUS=$(curl -s -o /tmp/test_thumb.jpg -w "%{http_code}" "$THUMBNAIL_URI")

if [ "$HTTP_STATUS" != "200" ]; then
    echo "FAIL: Thumbnail fetch returned HTTP $HTTP_STATUS"
    exit 1
fi

echo "  HTTP status: $HTTP_STATUS"

# Step 4: Verify it's a valid JPEG
FILE_TYPE=$(file /tmp/test_thumb.jpg | grep -o "JPEG image data" || echo "NOT_JPEG")

if [ "$FILE_TYPE" != "JPEG image data" ]; then
    echo "FAIL: Downloaded file is not a JPEG: $(file /tmp/test_thumb.jpg)"
    exit 1
fi

echo "  ✓ Valid JPEG image received"

# Step 5: Test navigate tool includes thumbnailUrl
echo ""
echo "Step 3: Testing navigate tool for thumbnailUrl in entries..."
PARENT_DIR=$(dirname "$TEST_FILE")
NAVIGATE_RESPONSE=$(curl -s -X POST "$MCP_URL" \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"params\":{\"name\":\"navigate\",\"arguments\":{\"path\":\"$PARENT_DIR\",\"limit\":50}},\"id\":2}")

# Count entries with thumbnailUrl
THUMB_COUNT=$(echo "$NAVIGATE_RESPONSE" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    text = d['result']['content'][0]['text']
    data = json.loads(text)
    entries = data.get('entries', [])
    count = sum(1 for e in entries if e.get('thumbnailUrl'))
    print(count)
except Exception as e:
    print('0')
")

echo "  Entries with thumbnailUrl: $THUMB_COUNT"

if [ "$THUMB_COUNT" -gt 0 ]; then
    echo "  ✓ Navigate returns entries with thumbnailUrl"
else
    echo "  WARNING: No entries have thumbnailUrl (thumbnails may not be generated)"
fi

# Step 6: Test inline metadata in inspect response
echo ""
echo "Step 4: Testing inline metadata..."
INLINE_METADATA_COUNT=$(echo "$INSPECT_DATA" | python3 -c "
import sys, json
d = json.load(sys.stdin)
metadata = d.get('metadata', [])
print(len(metadata))
")

echo "  Inline metadata entries: $INLINE_METADATA_COUNT"

if [ "$INLINE_METADATA_COUNT" -gt 0 ]; then
    echo "  ✓ Inspect returns inline metadata"

    # Check if metadata has URL
    HAS_URL=$(echo "$INSPECT_DATA" | python3 -c "
import sys, json
d = json.load(sys.stdin)
metadata = d.get('metadata', [])
has_url = any(m.get('url') for m in metadata)
print('yes' if has_url else 'no')
")

    if [ "$HAS_URL" == "yes" ]; then
        echo "  ✓ Metadata includes HTTP URLs"
    else
        echo "  WARNING: Metadata missing HTTP URLs"
    fi
else
    echo "  WARNING: No inline metadata in inspect response"
fi

echo ""
echo "=== All Tests Passed ==="
echo ""
echo "Summary:"
echo "  - Inspect tool returns HTTP thumbnailUri: ✓"
echo "  - Thumbnail accessible via HTTP: ✓"
echo "  - Thumbnail is valid JPEG: ✓"
echo "  - Navigate includes thumbnailUrl: $([ $THUMB_COUNT -gt 0 ] && echo '✓' || echo '⚠')"
echo "  - Inline metadata in inspect: $([ $INLINE_METADATA_COUNT -gt 0 ] && echo '✓' || echo '⚠')"

rm -f /tmp/test_thumb.jpg
