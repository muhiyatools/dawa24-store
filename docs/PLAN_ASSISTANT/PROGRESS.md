# AI Assistant — Progress

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| 1 | Credentials reach the Gateway | done | — | T1.1-T1.4 | `config_source.go` dynamic settings resolution with 30s cache TTL |
| 1 | Remove the provider-name leak | done | — | T1.5 | `check-provider-isolation` passes 100%, migration 102 applied |
| 2 | Streaming chat | done | — | T2.1-T2.5 | `chat_stream.go` with incremental SSE decoding, token streaming, cancellation |
| 2 | Multimodal content parts | done | — | T2.6 | OpenAI dialect parts (`image_url`, `input_audio`, `video_url`, `file`) |
| 2 | Transcription endpoint | done | — | T2.2 | `transcribe.go` with 10MB client guard and Whisper parameters |
| 2 | Capabilities fetch + cache | done | — | T2.7 | `capabilities.go` with runtime `/v1/models` fetching and 5m cache |
| 3 | Assistant module + routing | done | — | T3.1-T3.9 | `routing.go` multimodal pre-pass pipeline and digest generation |
| 3 | System prompt | done | — | T3.10 | Versioned prompt `2026-08-20.1` with medical safety + no system access bounds |
| 4 | SSE endpoint + cancellation | done | — | T4.1-T4.5 | `POST /api/v1/assistant/messages` SSE streaming with flusher |
| 4 | Rate limiting | done | — | T4.3 | Sliding window 20 req/min per user with 429 response |
| 5 | Attachment validation + lifecycle | done | — | T5.1-T5.5 | MIME sniffing, size caps, opaque user-bound handles |
| 6 | Native STT + Whisper fallback | done | — | T6.1-T6.4 | `POST /api/v1/assistant/transcribe` with Whisper-1 |
| 7 | Persistence + RLS | done | — | T7.1-T7.4 | Migration 103 with FORCE RLS + PostgreSQL repository |
| 8 | Frontend rebuild (B1-B6) | done | — | T8.1-T8.5 | Replaced simulated setTimeout with real SSE, uploads, speech recognition, and abort controllers |
| 9 | Performance + security verification | done | — | T1-T8 | Complete test suite passes with zero provider leaks and zero key exposure |
