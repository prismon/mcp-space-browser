# MCP Disk Indexer Web Component

A reusable web component microfrontend for filesystem indexing via the MCP Space Browser API.

## Overview

This directory contains a standalone web component that can be embedded in any HTML page to provide a UI for triggering disk indexing operations. The component is built using native Web Components API (Custom Elements) with Shadow DOM for full encapsulation.

## Files

- `mcp-disk-indexer.js` - The web component implementation
- `index.html` - Demo page showing usage examples
- `README.md` - This file

## Quick Start

1. Start the mcp-space-browser server:
   ```bash
   go run ./cmd/mcp-space-browser server --port=3000
   ```

2. Open the demo page in your browser:
   ```
   http://localhost:3000/web/index.html
   ```

3. Enter a filesystem path and click "Start Indexing"

## Usage

### Basic Usage

Include the script and use the custom element:

```html
<!DOCTYPE html>
<html>
<head>
  <title>My Page</title>
</head>
<body>
  <!-- Include the web component script -->
  <script src="http://localhost:3000/web/mcp-disk-indexer.js"></script>

  <!-- Use the component -->
  <mcp-disk-indexer></mcp-disk-indexer>
</body>
</html>
```

### With Attributes

```html
<mcp-disk-indexer
  api-base="http://localhost:3000"
  default-path="/home/user/Documents">
</mcp-disk-indexer>
```

### Listening to Events

```html
<script>
  const indexer = document.querySelector('mcp-disk-indexer');

  // Listen for successful indexing start
  indexer.addEventListener('index-started', (event) => {
    console.log('Indexing started for:', event.detail.path);
    console.log('Server response:', event.detail.result);
  });

  // Listen for errors
  indexer.addEventListener('index-error', (event) => {
    console.error('Error indexing:', event.detail.path);
    console.error('Error message:', event.detail.error);
  });
</script>
```

## API

### Attributes

| Attribute | Type | Default | Description |
|-----------|------|---------|-------------|
| `api-base` | string | `window.location.origin` | Base URL for the MCP Space Browser API |
| `default-path` | string | `/tmp` | Default filesystem path to show in the input field |
| `poll-progress` | boolean | `true` | Enable automatic job progress polling |

### Events

The component emits custom events that bubble up through the DOM:

#### `index-started`

Fired when indexing successfully starts.

```javascript
event.detail = {
  path: string,      // The path being indexed
  jobId: number,     // MCP job identifier
  status: string,    // Initial job status ('pending')
  response: object   // Full MCP response
}
```

#### `index-completed`

Fired when indexing completes successfully (only if `poll-progress` is enabled).

```javascript
event.detail = {
  jobId: number,     // Job identifier
  progress: object   // Job progress data
}
```

#### `index-failed`

Fired when indexing fails (only if `poll-progress` is enabled).

```javascript
event.detail = {
  jobId: number,     // Job identifier
  progress: object   // Job progress data
}
```

#### `index-error`

Fired when an error occurs during indexing.

```javascript
event.detail = {
  path: string,    // The path that failed
  error: string    // Error message
}
```

## Features

- **MCP Native**: Uses Model Context Protocol (JSON-RPC 2.0) for communication
- **Async Job Tracking**: Creates jobs and polls for progress automatically
- **Progress Visualization**: Real-time progress bar with percentage updates
- **Fully Encapsulated**: Uses Shadow DOM for complete style isolation
- **Reusable**: Can be dropped into any HTML page
- **Framework Agnostic**: Pure Web Components, works with any framework
- **Event-Driven**: Emits custom events for integration
- **Responsive**: Adapts to container width
- **Modern UI**: Clean, GitHub-inspired design

## Architecture

The component follows the Custom Elements v1 specification:

1. **Custom Element**: `<mcp-disk-indexer>` registered via `customElements.define()`
2. **Shadow DOM**: All styles and markup encapsulated
3. **Lifecycle Hooks**: `connectedCallback()` for initialization, `disconnectedCallback()` for cleanup
4. **Observed Attributes**: `attributeChangedCallback()` for reactive updates
5. **Event Emitters**: Custom events for external communication
6. **MCP Client**: JSON-RPC 2.0 client for MCP tool calls

## MCP Protocol

The component communicates via the Model Context Protocol (MCP) using JSON-RPC 2.0:

### Index Tool

Creates an asynchronous indexing job:

```
POST /mcp
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "index",
    "arguments": {
      "root": "/path/to/index",
      "async": true
    }
  }
}
```

Response:
```json
{
  "jobId": 123,
  "root": "/path/to/index",
  "status": "pending",
  "statusUrl": "shell://jobs/123"
}
```

### Job Progress Tool

Polls for job status and progress:

```
POST /mcp
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "job-progress",
    "arguments": {
      "jobId": "123"
    }
  }
}
```

Response:
```json
{
  "jobId": 123,
  "status": "running",
  "path": "/path/to/index",
  "progress": 45
}
```

## Browser Compatibility

Works in all modern browsers that support:
- Custom Elements v1
- Shadow DOM v1
- ES6 Classes
- Fetch API

Supported browsers:
- Chrome/Edge 67+
- Firefox 63+
- Safari 10.1+

## Development

To modify the component:

1. Edit `mcp-disk-indexer.js`
2. Refresh `index.html` to see changes
3. No build step required - native browser APIs

## Integration Examples

### React

```jsx
import { useEffect, useRef } from 'react';

function DiskIndexer() {
  const ref = useRef(null);

  useEffect(() => {
    const handleStart = (e) => console.log('Started:', e.detail);
    const handleError = (e) => console.error('Error:', e.detail);

    ref.current?.addEventListener('index-started', handleStart);
    ref.current?.addEventListener('index-error', handleError);

    return () => {
      ref.current?.removeEventListener('index-started', handleStart);
      ref.current?.removeEventListener('index-error', handleError);
    };
  }, []);

  return (
    <mcp-disk-indexer
      ref={ref}
      api-base="http://localhost:3000"
      default-path="/tmp"
    />
  );
}
```

### Vue

```vue
<template>
  <mcp-disk-indexer
    :api-base="apiBase"
    :default-path="defaultPath"
    @index-started="onIndexStarted"
    @index-error="onIndexError"
  />
</template>

<script>
export default {
  data() {
    return {
      apiBase: 'http://localhost:3000',
      defaultPath: '/tmp'
    };
  },
  methods: {
    onIndexStarted(event) {
      console.log('Started:', event.detail);
    },
    onIndexError(event) {
      console.error('Error:', event.detail);
    }
  }
};
</script>
```

### Angular

```typescript
import { Component, ViewChild, ElementRef, AfterViewInit } from '@angular/core';

@Component({
  selector: 'app-indexer',
  template: `
    <mcp-disk-indexer
      #indexer
      api-base="http://localhost:3000"
      default-path="/tmp">
    </mcp-disk-indexer>
  `
})
export class IndexerComponent implements AfterViewInit {
  @ViewChild('indexer') indexer!: ElementRef;

  ngAfterViewInit() {
    this.indexer.nativeElement.addEventListener('index-started', (e: any) => {
      console.log('Started:', e.detail);
    });

    this.indexer.nativeElement.addEventListener('index-error', (e: any) => {
      console.error('Error:', e.detail);
    });
  }
}
```

## License

MIT - Part of the [mcp-space-browser](https://github.com/prismon/mcp-space-browser) project.
