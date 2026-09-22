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
