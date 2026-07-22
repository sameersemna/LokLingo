package jobs

import "time"

const PDFUploadDir = "/tmp/loklingo"
const ImageUploadDir = "/tmp/loklingo/images"
const pdfTranslateConcurrency = 3
const translateMaxRetries = 3
var translateChunkTimeout = 25 * time.Second
var translateRetryBase = 500 * time.Millisecond
var slowChunkThreshold = 15 * time.Second
const pdfChunkMinWords = 80
const pdfChunkMaxWords = 150
const pdfChunkSep = "\n\n[[[LK_PAGE_BREAK]]]\n\n"
const defaultJobMaxAttempts = 3
const maxRetryDelay = 60 * time.Second
const retryJitterFraction = 0.25
const queueOverloadDepthThreshold = int64(200)
const retryBacklogOverloadThreshold = int64(120)
const stuckJobsOverloadThreshold = int64(20)
const severeQueueOverloadDepthThreshold = int64(400)
const severeRetryBacklogThreshold = int64(240)
const severeStuckJobsThreshold = int64(40)
const lowQueueDepthThreshold = int64(30)
const lowRetryBacklogThreshold = int64(10)
const adaptiveTranslateConcurrencyCeiling = 6
const lowOCRConfidenceThreshold = 0.72
const minAdaptiveChunkWordsFloor = 40
