import '@testing-library/jest-dom'

// jsdom does not fully implement Blob.prototype.arrayBuffer on sliced blobs.
// FileReader is the only reliable fallback in jsdom: Response.arrayBuffer() delegates
// back to Blob.arrayBuffer() internally, creating a circular dependency when the
// native method is absent.
if (typeof Blob !== 'undefined') {
  const proto = Blob.prototype as Blob & { arrayBuffer?: () => Promise<ArrayBuffer> }
  if (!proto.arrayBuffer) {
    proto.arrayBuffer = function (this: Blob): Promise<ArrayBuffer> {
      return new Promise<ArrayBuffer>((resolve, reject) => {
        const reader = new FileReader()
        reader.addEventListener('load', () => resolve(reader.result as ArrayBuffer))
        reader.addEventListener('error', () => reject(reader.error ?? new Error('FileReader error')))
        reader.readAsArrayBuffer(this) // NOSONAR — Blob#arrayBuffer() cannot be used here: it would recurse into this same polyfill
      })
    }
  }
}
