import '@testing-library/jest-dom'

// jsdom does not fully implement Blob.prototype.arrayBuffer on sliced blobs.
// Polyfill using FileReader (available in jsdom ≥ 20).
if (typeof Blob !== 'undefined') {
  const proto = Blob.prototype as Blob & { arrayBuffer?: () => Promise<ArrayBuffer> }
  if (!proto.arrayBuffer) {
    proto.arrayBuffer = function (this: Blob): Promise<ArrayBuffer> {
      return new Promise((resolve, reject) => {
        const reader = new FileReader()
        reader.addEventListener('load', () => resolve(reader.result as ArrayBuffer))
        reader.addEventListener('error', () => reject(reader.error))
        reader.readAsArrayBuffer(this)
      })
    }
  }
}
