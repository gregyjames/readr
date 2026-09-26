import { GlobalWindow } from 'happy-dom'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import { plugin } from 'bun'

// 1. Initialize DOM globals via happy-dom
const win = new GlobalWindow()
let curr: object | null = win
while (curr && curr !== Object.prototype) {
  for (const key of Object.getOwnPropertyNames(curr)) {
    if (!(key in globalThis)) {
      try {
        const desc = Object.getOwnPropertyDescriptor(curr, key)
        if (desc && typeof desc.value !== 'undefined') {
          (globalThis as Record<string, unknown>)[key] = desc.value
        } else if (desc && desc.get) {
          Object.defineProperty(globalThis, key, desc)
        } else {
          (globalThis as Record<string, unknown>)[key] = (win as unknown as Record<string, unknown>)[key]
        }
      } catch {
        // Ignore unconfigurable or read-only properties
      }
    }
  }
  curr = Object.getPrototypeOf(curr)
}
globalThis.window = win as unknown as Window & typeof globalThis
globalThis.document = win.document as unknown as Document
globalThis.navigator = win.navigator as unknown as Navigator

// Ensure window.location has a valid HTTP origin for relative axios/fetch requests
if (typeof window !== 'undefined' && (!window.location.origin || window.location.origin === 'null')) {
  try {
    window.location.href = 'http://localhost:5173/'
  } catch {
    // Ignore if location is read-only
  }
}

// Suppress expected unmocked network error logs during component mount tests
const originalConsoleError = console.error
console.error = (...args: unknown[]) => {
  const first = typeof args[0] === 'string' ? args[0] : ''
  if (
    first.includes('Failed to load local graph') ||
    first.includes('Failed to load graph') ||
    first.includes('Failed to save reading progress') ||
    first.includes('Search failed') ||
    first.includes('Failed to fetch templates')
  ) {
    return
  }
  originalConsoleError(...args)
}

const originalConsoleWarn = console.warn
console.warn = (...args: unknown[]) => {
  const first = typeof args[0] === 'string' ? args[0] : ''
  if (first.includes('EventSource failed')) {
    return
  }
  originalConsoleWarn(...args)
}

// Mock EventSource for Server-Sent Events tests
if (typeof EventSource === 'undefined') {
  class MockEventSource {
    url: string
    onmessage: ((event: { data: string }) => void) | null = null
    onerror: (() => void) | null = null
    constructor(url: string) {
      this.url = url
    }
    close() {}
  }
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource
}

// Polyfill Node.prototype.nodeName getter so DOMPurify accurately identifies element and text nodes in happy-dom
Object.defineProperty(Node.prototype, 'nodeName', {
  get(this: Node) {
    switch (this.nodeType) {
      case 1:
        return (this as Element).tagName || ''
      case 3:
        return '#text'
      case 8:
        return '#comment'
      case 9:
        return '#document'
      case 11:
        return '#document-fragment'
      default:
        return ''
    }
  },
  configurable: true,
})

// Mock CanvasRenderingContext2D for headless vis-network graph tests
if (typeof HTMLCanvasElement !== 'undefined') {
  HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, type: string, ..._rest: unknown[]) {
    if (type === '2d') {
      return {
        canvas: this,
        save: () => {},
        restore: () => {},
        scale: () => {},
        rotate: () => {},
        translate: () => {},
        transform: () => {},
        setTransform: () => {},
        resetTransform: () => {},
        clearRect: () => {},
        fillRect: () => {},
        strokeRect: () => {},
        beginPath: () => {},
        closePath: () => {},
        moveTo: () => {},
        lineTo: () => {},
        arc: () => {},
        fill: () => {},
        stroke: () => {},
        measureText: (text: string) => ({ width: (text || '').length * 8 }),
        fillText: () => {},
        strokeText: () => {},
        drawImage: () => {},
        createLinearGradient: () => ({ addColorStop: () => {} }),
        createRadialGradient: () => ({ addColorStop: () => {} }),
        createPattern: () => null,
        getImageData: () => ({ data: [] }),
        putImageData: () => {},
      } as unknown as CanvasRenderingContext2D
    }
    return null
  } as unknown as typeof HTMLCanvasElement.prototype.getContext
}

// 2. Register Bun loader plugin for .vue SFC files
plugin({
  name: 'vue-sfc-loader',
  setup(build) {
    build.onLoad({ filter: /\.vue$/ }, async (args) => {
      const source = await Bun.file(args.path).text()
      const { descriptor } = parse(source, { filename: args.path })
      const id = Math.random().toString(36).substring(2, 8)
      let scriptCode = 'export default {}'
      if (descriptor.script || descriptor.scriptSetup) {
        const script = compileScript(descriptor, { id, inlineTemplate: true })
        scriptCode = script.content
      } else if (descriptor.template) {
        const template = compileTemplate({
          source: descriptor.template.content,
          filename: args.path,
          id,
        })
        scriptCode = template.code + '\nexport default { render }'
      }
      return {
        contents: scriptCode,
        loader: 'ts',
      }
    })
  },
})
