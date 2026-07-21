#!/usr/bin/env node

import { createReadStream } from 'node:fs'
import { stat } from 'node:fs/promises'
import { basename, extname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const contentTypes = {
  '.avif': 'image/avif',
  '.csv': 'text/csv',
  '.gif': 'image/gif',
  '.jpeg': 'image/jpeg',
  '.jpg': 'image/jpeg',
  '.json': 'application/json',
  '.m4a': 'audio/mp4',
  '.md': 'text/markdown',
  '.mov': 'video/quicktime',
  '.mp3': 'audio/mpeg',
  '.mp4': 'video/mp4',
  '.pdf': 'application/pdf',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.txt': 'text/plain',
  '.wav': 'audio/wav',
  '.webm': 'video/webm',
  '.webp': 'image/webp',
  '.zip': 'application/zip',
}

function inferContentType(filePath) {
  return contentTypes[extname(filePath).toLowerCase()] ?? 'application/octet-stream'
}

function parseJsonResponse(text, source) {
  try {
    return JSON.parse(text)
  } catch {
    throw new Error(`${source} returned invalid JSON`)
  }
}

function responseError(response, body, source) {
  const message = body?.error?.message || body?.message || response.statusText
  return new Error(`${source} failed (${response.status}): ${message}`)
}

export async function uploadFile({
  baseURL,
  apiKey,
  filePath,
  contentType,
  uploadTimeoutMs = 10 * 60 * 1000,
}) {
  if (!baseURL) {
    throw new Error('baseURL is required')
  }
  if (!apiKey) {
    throw new Error('apiKey is required')
  }
  if (!filePath) {
    throw new Error('filePath is required')
  }
  if (!Number.isSafeInteger(uploadTimeoutMs) || uploadTimeoutMs < 1) {
    throw new Error('uploadTimeoutMs must be a positive integer')
  }

  const absoluteFilePath = resolve(filePath)
  const fileStat = await stat(absoluteFilePath)
  if (!fileStat.isFile()) {
    throw new Error(`not a regular file: ${absoluteFilePath}`)
  }

  const newAPIURL = new URL(baseURL)
  if (!['http:', 'https:'].includes(newAPIURL.protocol)) {
    throw new Error('baseURL must use HTTP or HTTPS')
  }
  const isLocalhost = ['localhost', '127.0.0.1', '[::1]'].includes(newAPIURL.hostname)
  if (newAPIURL.protocol !== 'https:' && !isLocalhost) {
    throw new Error('baseURL must use HTTPS unless it points to localhost')
  }

  const resolvedContentType = contentType || inferContentType(absoluteFilePath)
  const authorization = /^Bearer\s/i.test(apiKey) ? apiKey : `Bearer ${apiKey}`
  const presignURL = new URL('/v1/upload/presign', newAPIURL)
  const presignResponse = await fetch(presignURL, {
    method: 'POST',
    headers: {
      Authorization: authorization,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      filename: basename(absoluteFilePath),
      content_type: resolvedContentType,
    }),
    redirect: 'error',
    signal: AbortSignal.timeout(15_000),
  })
  const presignBody = parseJsonResponse(await presignResponse.text(), 'NewAPI presign endpoint')
  if (!presignResponse.ok) {
    throw responseError(presignResponse, presignBody, 'NewAPI presign endpoint')
  }

  const {
    url,
    public_url: publicUrl,
    method,
    object_key: objectKey,
    headers,
    expires_at: expiresAt,
  } = presignBody
  if (!url || !publicUrl || method !== 'PUT' || !objectKey || !headers || !expiresAt) {
    throw new Error('NewAPI presign endpoint returned an incomplete response')
  }

  const uploadURL = new URL(url)
  if (!['http:', 'https:'].includes(uploadURL.protocol)) {
    throw new Error('pre-signed upload URL must use HTTP or HTTPS')
  }

  const uploadHeaders = new Headers(headers)
  uploadHeaders.set('Content-Length', String(fileStat.size))
  const uploadResponse = await fetch(uploadURL, {
    method,
    headers: uploadHeaders,
    body: createReadStream(absoluteFilePath),
    duplex: 'half',
    redirect: 'error',
    signal: AbortSignal.timeout(uploadTimeoutMs),
  })
  const uploadResponseBody = await uploadResponse.text()
  if (!uploadResponse.ok) {
    throw new Error(`TOS upload failed (${uploadResponse.status}): ${uploadResponseBody || uploadResponse.statusText}`)
  }
  let uploadRes = null
  if (uploadResponseBody) {
    try {
      uploadRes = JSON.parse(uploadResponseBody)
    } catch {
      uploadRes = uploadResponseBody
    }
  }

  return {
    objectKey,
    publicUrl,
    size: fileStat.size,
    contentType: uploadHeaders.get('Content-Type') || resolvedContentType,
    expiresAt,
    uploadRes,
    presignBody,
  }
}

function printUsage() {
  console.log(`Usage:
  node scripts/tos-upload-client.mjs --file <path> [options]

Options:
  --file <path>          File to upload (or use the first positional argument)
  --base-url <url>       NewAPI address (default: NEW_API_BASE_URL or http://localhost:3000)
  --api-key <key>        NewAPI key (default: NEW_API_KEY)
  --content-type <type>  Override the inferred content type
  --timeout-ms <number>  Upload timeout in milliseconds (default: 600000)
  --help                 Show this help`)
}

function parseArguments(args) {
  const options = {}
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index]
    if (argument === '--help') {
      options.help = true
      continue
    }
    if (!argument.startsWith('--') && !options.filePath) {
      options.filePath = argument
      continue
    }

    const value = args[index + 1]
    if (!value || value.startsWith('--')) {
      throw new Error(`missing value for ${argument}`)
    }
    index += 1
    switch (argument) {
      case '--file':
        options.filePath = value
        break
      case '--base-url':
        options.baseURL = value
        break
      case '--apikey':
        options.apiKey = value
        break
      case '--content-type':
        options.contentType = value
        break
      case '--timeout-ms':
        options.uploadTimeoutMs = Number(value)
        break
      default:
        throw new Error(`unknown option: ${argument}`)
    }
  }
  return options
}

async function main() {
  const options = parseArguments(process.argv.slice(2))
  if (options.help) {
    printUsage()
    return
  }

  const result = await uploadFile({
    baseURL: options.baseURL || process.env.NEW_API_BASE_URL || 'http://localhost:3000',
    apiKey: options.apiKey || process.env.NEW_API_KEY,
    filePath: options.filePath,
    contentType: options.contentType,
    uploadTimeoutMs: options.uploadTimeoutMs,
  })
  console.log(JSON.stringify(result, null, 2))
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error.message)
    process.exitCode = 1
  })
}
