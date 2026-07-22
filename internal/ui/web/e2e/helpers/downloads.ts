import { expect } from '@playwright/test'
import { gunzipSync, gzipSync } from 'node:zlib'

const ZIP_LOCAL_HEADER = 0x04034b50

export function mockGzipArchive(payload = 'mock lab backup payload\n'): Buffer {
  return gzipSync(Buffer.from(payload))
}

/** Minimal ZIP with one safe entry (hello.txt). */
export function mockZipArchive(): Buffer {
  return Buffer.from(
    'UEsDBAoAAAAAAJBW9lyGphA2BQAAAAUAAAAJABwAaGVsbG8udHh0VVQJAAOvaGBqr2hganV4CwABBPYBAAAEFAAAAGhlbGxvUEsBAh4DCgAAAAAAkFb2XIamEDYFAAAABQAAAAkAGAAAAAAAAQAAAKSBAAAAAGhlbGxvLnR4dFVUBQADr2hganV4CwABBPYBAAAEFAAAAFBLBQYAAAAAAQABAE8AAABIAAAAAAA=',
    'base64',
  )
}

function listZipEntryNames(data: Buffer): string[] {
  const names: string[] = []
  let offset = 0
  while (offset + 30 <= data.length) {
    const signature = data.readUInt32LE(offset)
    if (signature !== ZIP_LOCAL_HEADER) break
    const nameLength = data.readUInt16LE(offset + 26)
    const extraLength = data.readUInt16LE(offset + 28)
    const nameStart = offset + 30
    const nameEnd = nameStart + nameLength
    if (nameEnd > data.length) break
    names.push(data.subarray(nameStart, nameEnd).toString('utf8'))
    const compressedSize = data.readUInt32LE(offset + 18)
    offset = nameEnd + extraLength + compressedSize
  }
  return names
}

export function assertSafeGzip(buffer: Buffer): void {
  expect(buffer.length).toBeGreaterThan(2)
  expect(buffer[0]).toBe(0x1f)
  expect(buffer[1]).toBe(0x8b)
  const inflated = gunzipSync(buffer)
  expect(inflated.length).toBeGreaterThan(0)
}

export function assertSafeZip(buffer: Buffer): void {
  expect(buffer.length).toBeGreaterThan(4)
  expect(buffer.readUInt32LE(0)).toBe(ZIP_LOCAL_HEADER)
  const entries = listZipEntryNames(buffer)
  expect(entries.length).toBeGreaterThan(0)
  for (const name of entries) {
    expect(name).not.toMatch(/^\.\./)
    expect(name).not.toMatch(/^[/\\]/)
    expect(name).not.toContain('..\\')
    expect(name).not.toContain('../')
  }
}
