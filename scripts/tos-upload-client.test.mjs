import { createServer } from 'node:http'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { once } from 'node:events'
import test from 'node:test'
import assert from 'node:assert/strict'

import { uploadFile } from './tos-upload-client.mjs'

test('requests a pre-signed URL and uploads the file', async (t) => {
  const directory = await mkdtemp(join(tmpdir(), 'tos-upload-client-'))
  const filePath = join(directory, 'sample.txt')
  await writeFile(filePath, 'hello from the NewAPI client')
  t.after(() => rm(directory, { recursive: true, force: true }))

  let uploadedBody = ''
  const server = createServer(async (request, response) => {
    if (request.method === 'POST' && request.url === '/v1/upload/presign') {
      assert.equal(request.headers.authorization, 'Bearer sk-test')
      let body = ''
      for await (const chunk of request) body += chunk
      assert.deepEqual(JSON.parse(body), {
        filename: 'sample.txt',
        content_type: 'text/plain',
      })

      const address = server.address()
      response.setHeader('Content-Type', 'application/json')
      response.end(JSON.stringify({
        url: `http://127.0.0.1:${address.port}/tos/object`,
        public_url: 'https://test-bucket.tos.example.com/uploads/7/sample.txt',
        method: 'PUT',
        object_key: 'uploads/7/sample.txt',
        headers: { 'Content-Type': 'text/plain' },
        expires_at: 2_000_000_000,
      }))
      return
    }

    if (request.method === 'PUT' && request.url === '/tos/object') {
      assert.equal(request.headers['content-type'], 'text/plain')
      for await (const chunk of request) uploadedBody += chunk
      response.statusCode = 200
      response.end()
      return
    }

    response.statusCode = 404
    response.end()
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  t.after(() => server.close())

  const address = server.address()
  const result = await uploadFile({
    baseURL: `http://127.0.0.1:${address.port}`,
    apiKey: 'sk-test',
    filePath,
  })

  assert.equal(uploadedBody, 'hello from the NewAPI client')
  assert.deepEqual(result, {
    objectKey: 'uploads/7/sample.txt',
    publicUrl: 'https://test-bucket.tos.example.com/uploads/7/sample.txt',
    size: 28,
    contentType: 'text/plain',
    expiresAt: 2_000_000_000,
    uploadRes: null,
    presignBody: {
      url: `http://127.0.0.1:${address.port}/tos/object`,
      public_url: 'https://test-bucket.tos.example.com/uploads/7/sample.txt',
      method: 'PUT',
      object_key: 'uploads/7/sample.txt',
      headers: { 'Content-Type': 'text/plain' },
      expires_at: 2_000_000_000,
    },
  })
})
