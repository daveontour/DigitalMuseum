# How I Used Five Different AI Providers — and Thirteen Different AI Roles — in a Single Personal Archive App

*A look at what happens when you let AI loose on a lifetime of emails, messages, and photos.*

---

Digital Museum started as a small utility with a specific purpose: to understand in depth how LLM APIs actually work. The first version was a Python script. As the scope grew, I migrated it to Go for performance and portability, and it eventually became a full Electron desktop app that aggregates your emails, messages, photos, Facebook exports, and iMessage history into a single searchable database, then lets you explore it through AI.

There's a parallel story running alongside the product itself: the way I wrote the code changed dramatically over the life of the project. What began with simple inline code completion evolved into fully autonomous agentic coding — AI not just suggesting the next line, but designing, implementing, and refactoring entire features end-to-end. The project became as much an experiment in how to build with AI as it was an experiment in what to build with AI.

What started as a simple chat interface quickly grew into something I didn't fully anticipate: a project where AI isn't just one feature, but the architectural backbone. Along the way I ended up integrating five different AI providers and finding thirteen meaningfully distinct ways to put them to work.

Here's what I learned.

---

## The Thirteen Roles AI Plays

When I step back and look at how AI is actually used in the app, I count thirteen distinct jobs — and they pull in very different directions.

### 1. Archivist

The core function. You ask a question — "What did I write to Mum on her 70th birthday?" or "Find all the times I discussed my redundancy in 2018" — and the AI calls database tools to search your emails, messages, Facebook posts, and photo metadata, then synthesises an answer. It has structured access to your data through a tool-calling layer, not just a keyword search.

### 2. Voice Personality

The AI doesn't have to sound like an AI. I built a voice system where you choose how it responds: as a formal expert, a warm friend, in a therapeutic register, or — most interestingly — in the voice of the archive subject themselves. The system extracts writing samples from the person's actual emails and messages to calibrate tone. You can also create fully custom personas with their own instructions and creativity levels.

### 3. Two-Voice Conversationalist

One of the stranger features: **Have-a-Chat** sets two AI instances talking to each other *about* the archive subject. They're given different personas and watch each other's responses unfold. It produces conversations that are surprisingly revealing, and occasionally surreal.

### 4. Interviewer

The app can conduct a structured life-story interview with the archive owner. The AI asks questions, listens to answers, and then — when the session is marked finished — generates a prose narrative from the full transcript. You can set the style: formal, journalistic, therapeutic, or casual.

### 5. Memory Companion

**Pam Bot** is a gentler mode built specifically for people with dementia or memory difficulties. It asks warm, simple questions about familiar photos and people, maintains conversational continuity across sessions, and analyses recurring topics over time. It never corrects or challenges — it only encourages and reflects. This is the feature I'm most careful about: the system prompt for this mode is completely separate from the main chat, and the tool set it can access is deliberately limited.

### 6. Profile Builder

Given enough emails, messages, and posts, the AI can assemble a rich textual portrait of the archive subject — their writing style, their inferred personality, their recurring preoccupations. This runs as an explicit wizard rather than happening automatically, but the results feed back into every subsequent interaction.

### 7. Image Classifier and Tagger

Every photo in the archive can be passed to an AI vision model for automatic content classification. The AI looks at the image and returns a set of descriptive tags — people, places, objects, moods, occasions. These tags are stored alongside the photo's metadata and are immediately searchable and filterable through the gallery UI.

But here's where it gets interesting: the tags don't just sit in a text column. They're also **vectorised** and stored alongside the image in the embedding layer, so a semantic similarity search can find images by concept rather than exact keyword. Searching for "birthday celebration" can surface photos tagged "cake," "candles," and "family gathering" — even if none of those words appeared in the query.

### 8. Similarity Search

Not all AI use in the app involves conversation. I run an **embedding model** (via a separate Ollama instance) over messages, emails, and image tags, then store those vectors in SQLite using sqlite-vec. This powers semantic similarity search across the entire archive — you can find content that is conceptually related to a query even if it shares no keywords with it.

### 9. Question Generator

A small but high-value feature: the AI generates a contextually-aware random question based on the subject's profile and interests, then answers it from the archive. It's a daily prompt for exploration. The best ones surface things you'd completely forgotten.

### 10. Summariser

Conversations and email threads can be summarised on demand — useful when you want the shape of a long exchange without reading hundreds of messages. I also use AI to generate quick and detailed summaries of reference documents that have been uploaded to the system.

### 11. Reference Librarian

Users can upload documents — PDFs, Word files, text — and mark them as reference material. The AI can retrieve these on demand during chat, and documents marked as "always include" are injected directly into the system prompt for every conversation. Think of it as giving the AI a bookshelf it can pull from.

### 12. Writing Coach

The app can analyse the archive subject's emails and produce a formal description of their writing style — sentence length, vocabulary, tone, tendencies. This feeds the voice mimicry system described above, but also stands alone as a piece of self-knowledge.

### 13. Web Researcher

Via the Tavily API, the AI can reach outside the archive and search the live web when a question needs current context that the personal archive can't provide.

---

## The Data Problem: An AI Archive Is Nothing Without the Archive

The AI components are the most visible part of Digital Museum, and the most interesting to write about. But they rest on something far less glamorous that took a disproportionate share of the total development effort: getting the data in.

Without the data, the app is an intelligent shell talking to an empty database. The AI can call its tools perfectly, reason flawlessly, handle every edge case in the tool-calling loop — and return nothing, because there is nothing to find. The quality and completeness of the archive is the single biggest determinant of whether the experience is useful or hollow.

### Seven sources, seven entirely different problems

The import pipeline covers seven distinct data sources. Each one required its own parser, its own data model mapping, its own approach to deduplication, and its own handling of whatever format quirks the exporting platform had chosen to impose.

**Facebook** exports data as a deeply nested JSON structure spread across dozens of files in a ZIP archive. The challenge is not the structure — it's the encoding. Facebook historically encoded text using a Latin-1 byte sequence that was then interpreted as UTF-8, meaning a name like "André" would arrive in the export as something visually wrong and need fixing during import. Every string in the Facebook parser goes through a decoding step before it touches the database.

**Instagram** follows a similar export format but with different file layouts, different field names, and subtly different media attachment conventions. A separate parser, despite the surface similarity.

**WhatsApp** exports via iMazing as CSV — a deceptively simple format that conceals significant variation. Message timestamps, sender identification, attachment references, and reply threading all needed careful parsing. The date format alone required handling multiple layout strings to cover variations across export versions.

**iMessage** arrives as another iMazing export, this time with conversations represented as directories of CSV files rather than a single flat file. The importer has to aggregate metadata across all files in a conversation folder to reconstruct the chat session correctly, handling edge cases where the same conversation appears under different session names across exports.

**IMAP email** is its own small project. The importer connects to arbitrary IMAP servers — not just Gmail — handles TLS, parses raw RFC 2822 message bytes, decodes MIME multipart bodies, handles quoted-printable and base64 encoding, deals with non-UTF-8 character sets (Latin-1, Windows-1252, and others), extracts attachments, and writes everything into the database with proper threading. The IMAP importer alone is 665 lines. Email is hard.

**Gmail via OAuth2** adds a credential management layer on top of all the above: a full OAuth2 flow with a redirect callback, token storage, refresh handling, and then the same IMAP-style message processing once connected.

**Filesystem images** are the most structurally simple and the most physically large. The importer walks directories recursively, reads EXIF metadata to extract date taken (using ImageMagick, because the EXIF date format — `2024:01:15 12:34:56` — is non-standard and requires dedicated parsing), generates thumbnails, extracts GPS coordinates, and handles the full range of supported image types.

### The contact problem

Every data source has its own identity system. Facebook knows people by their Facebook names. WhatsApp knows them by phone number. iMessage knows them by Apple ID or phone number. Email knows them by address. The same real person appears in the archive under four or five different identifiers, with no connection between them.

The contacts pipeline exists to solve this. It reads every identity across every source, applies a series of merge heuristics — exact email match, fuzzy name matching with a Jaccard similarity score, explicit manual mapping rules, and a lookup table of common name variations (Dave/David, Bob/Robert, Chris/Christopher) — and groups them into unified contact records. The threshold for automatic merging is set conservatively at 0.85 similarity to avoid false positives. Below that, contacts stay separate until manually reviewed.

This matters more than it might seem. When the AI calls `get_all_messages_by_contact` for "Alice," it needs to know that "Alice Smith," "alice@gmail.com," and the WhatsApp sender "+44 7700 900123" are the same person. Without that mapping, the tool returns a fragment. With it, it returns a complete picture of a relationship.

### Data preparation: what happens after import

Import is not the end of the pipeline — it's the beginning. Several preparation steps run after data is in the database before the AI can use it effectively.

**Thumbnail generation** runs ImageMagick against every imported photo to produce a smaller JPEG for gallery display and for the vision model's image classification step. The processor handles format conversion, orientation correction from EXIF, and error recovery for malformed image files.

**Embedding generation** runs every message and email through the local Ollama embedding model and stores the resulting vectors in sqlite-vec tables. This is the background job that makes semantic similarity search possible. For a large archive it runs for hours, processing items in batches and tracking which records have been embedded so reruns are incremental.

**Image classification** runs the vision model over every photo that is flagged as requiring classification, producing keyword tags that are then also vectorised. This is where the RunPod parallelism described earlier is most valuable — ten thousand photos through a single local model is a weekend job; the same ten thousand across a pool of parallel serverless workers is a coffee break.

### The honest assessment

The import pipeline is approximately 8,000 lines of Go across 44 files. That is more code than the AI integration, more code than the authentication system, more code than the handler and repository layers combined. It is also the least interesting code to read, the least likely to appear in a portfolio, and the hardest to test thoroughly because the input data is real-world messy in ways that synthetic fixtures don't capture.

But it is the foundation. The AI's ability to answer "what were we talking about in those WhatsApp messages from 2019?" depends entirely on the 665-line IMAP importer, the Facebook encoding fix, the iMazing CSV parser, and the contact merge heuristic getting it right first.

An intelligent archive is only as good as the archive.

---

## How the AI Accesses Your Data: The Tool Layer

The word "access" needs unpacking. The AI doesn't hold a copy of your archive. It doesn't read your emails into its training weights. When you ask it something, it has access to a defined set of **tools** — structured functions it can call to query the SQLite database — and it uses those tools to retrieve exactly what it needs to answer the question, in real time, scoped strictly to your data.

The loop works like this: you send a message → the AI decides which tool to call and with what arguments → the server runs the query → the result comes back as plain text → the AI reasons over it and either calls another tool or composes a final answer. This can repeat up to fifteen times per request. The AI is doing detective work, not recall.

Here is the full set of tools available, grouped by what they access:

**Messages and conversations** (WhatsApp, iMessage, SMS, Facebook Messenger)
- *List available chat sessions* — discover what conversations exist before trying to retrieve one
- *Get messages by chat session* — retrieve the full content of a named conversation
- *Get messages around a specific message* — zoom into context around an anchor point
- *Search messages globally* — keyword search across every conversation at once
- *Search messages within a session* — keyword search within one conversation
- *Search messages by similarity* — vector search: find messages conceptually close to a query, even without matching keywords

**Emails**
- *Get emails by contact* — all email to or from a named person or address
- *Get all communications by contact* — email and messages combined, for a full picture of a relationship
- *Search emails by similarity* — same vector search capability applied to the email archive

**Facebook**
- *Search Facebook albums* — find photo albums by keyword
- *Get album images* — retrieve the first images from a specific album (used to surface photos during conversation)
- *Search Facebook posts* — find posts matching a text description

**Media and artefacts**
- *Get unique tags and counts* — see what tags exist across photos and physical artefacts, useful for understanding what the archive contains

**Reference documents**
- *List available reference documents* — discover what documents the owner has uploaded
- *Get reference document* — retrieve the full content of one or more documents
- *List sensitive reference documents* — same, for private or encrypted documents (requires keyring unlock)
- *Get sensitive reference document* — retrieve encrypted document content (owner session only, or visitor with explicit permission)

**Archive subject**
- *Get user interests* — the recorded interests of the archive subject, used for contextual questions
- *Get writing examples* — samples of the subject's actual writing, used for voice calibration
- *List complete profiles* — discover what generated psychological and relationship profiles exist
- *Get complete profile* — retrieve the full text of a generated profile for a named person

**Interviews**
- *List interviews* — see what structured interview sessions have been conducted
- *Get interview* — retrieve the full question-and-answer transcript and any generated writeup

**Utility**
- *Get current time* — current UTC timestamp (more useful than it sounds when answering "what year were those messages from?")
- *Web search* — live internet search via Tavily, for questions the archive can't answer

### Two search strategies, used together

Most of the data-access tools use keyword or structured lookup. But messages and emails also have a parallel vector search path: `search_messages_by_similarity` and `search_emails_by_similarity`. These embed the query using the local embedding model and find content that is semantically close — useful when you don't know the exact words used, or when the concept matters more than the vocabulary.

In practice the AI will sometimes use both strategies on the same question: keyword search to find direct references, similarity search to find related content that doesn't use the same words.

### Access control baked into every tool call

Every tool call runs SQL that includes an explicit `AND user_id = ?` filter. There is no path through which the AI can see another user's data, even if it were somehow prompted to try. For visitor sessions, each tool is independently gated: owners configure which tools visitors can invoke, and sensitive document tools have an additional check for keyring unlock status before returning any content.

---

## Five Providers, Five Different Contracts

Running all of these roles required integrating five separate AI providers. Each has a different API shape, a different authentication model, and different trade-offs.

### Anthropic Claude

My primary provider for complex reasoning. Claude uses the Messages API with native tool-calling — the AI signals which tool it wants to invoke, the server runs the query, returns the result, and the loop continues until Claude has enough to answer. It supports prompt caching, which matters when you're injecting large system prompts on every turn.

### Google Gemini

Gemini's function-calling mechanism works similarly to Claude's but through a different API surface and authentication model (`key` query parameter rather than a header). I use the Gemini File API for uploaded reference documents — Gemini can hold a reference to an uploaded file across requests rather than re-sending the content each time.

### DeepSeek

DeepSeek exposes an **Anthropic-compatible** Messages API endpoint. In practice, this meant I could reuse almost all of my Claude integration code — just swap the base URL and authentication header. This was a deliberate choice by DeepSeek that made it very easy to add as an option. The tool-calling loop is identical.

### OpenAI ChatGPT

ChatGPT uses the Chat Completions API, which has a different message format, a different tool-calling structure (`tool_calls` rather than `tool_use`), and different token count fields. Worth having as an option, but the integration is genuinely a different code path.

### Local Ollama (Gemma4)

This one is the most interesting architecturally. For users who don't want their data touching any external service, or who just want to run everything on their own hardware, I bundle Ollama and launch it as a separate process. It runs locally, it's completely offline, and it supports the same tool-calling loop as the cloud providers — just with a native Ollama API rather than an HTTP JSON API.

I actually run **two** Ollama instances: one for chat (on port 11434) and a separate one for embeddings (on port 11435). The embedding model needs to stay warm without competing for GPU memory with the chat model.

---

## Provisioning: Who Controls Which Keys?

With five providers and multiple users, key management gets interesting. I built three layers:

**Server keys** — set in the environment file by whoever deploys the server. Available to all users by default, configurable to allow or deny per user.

**Per-user keys** — users can enter their own API keys through the settings panel. These are stored encrypted in the database and override server keys when present. The `allow_server_keys` flag lets a user opt out of shared infrastructure entirely.

**Visitor keys** — when you share archive access with a visitor (a family member browsing your photos, say), you can grant them their own LLM access with their own key overrides, without giving them any broader system access.

Beyond keys, there's a **tool access tier** system. Every AI tool — every database query the AI can invoke — is assigned a tier: owner-only or visitor-accessible. Owners can configure which tools their visitors can use, giving fine-grained control over what an AI is allowed to retrieve on a visitor's behalf.

---

## Scaling Image Classification with On-Demand Serverless Workers

Gemma4 running locally does a genuinely good job of classifying photo content. It can look at an image and return a meaningful set of keyword tags — people, places, objects, occasions — without sending a single byte to a cloud provider. For a handful of photos, local classification is fast enough.

For a lifetime of photos, it is not.

A personal archive can easily hold tens of thousands of images. Running them sequentially through a local model, one at a time, would take hours or days. The solution I reached for is **on-demand serverless GPU workers on RunPod**, running the same Gemma4 model, spinning up only when classification is triggered.

### The worker pool architecture

When image classification starts, the system launches a pool of concurrent goroutines pulling from a shared work queue. The pool is always a combination of two types:

**RunPod workers (N, configurable)** — each sends the image as base64-encoded JPEG to a serverless endpoint on RunPod via its `runsync` API. The endpoint runs the same Gemma4 model on GPU hardware and returns a JSON array of keyword tags. If RunPod fails for any reason, the worker automatically falls back to the local Ollama instance for that image.

**One local worker (always present)** — pulls from the same queue, classifying via local Ollama. If local Ollama fails on a given image, it falls back to RunPod. The local worker ensures classification continues even if RunPod is unavailable, and handles overflow when the RunPod workers are busy.

The result is a pipeline that saturates all available workers in parallel, with each worker independently resilient to the failure of its primary backend.

### Consistent results across backends

Because the same model runs everywhere — Gemma4 locally and Gemma4 on RunPod — the tag vocabulary is consistent. There's no divergence in classification style between images processed locally and images processed in the cloud. A tag that means "birthday party" means it in both places.

### On-demand cost

RunPod serverless workers have zero idle cost. The endpoint exists but incurs no charges until a request arrives. Triggering a classification batch starts the workers; when the queue is empty and the job completes, they go cold again. The cost of classifying ten thousand images is a one-time burst charge, not an ongoing subscription.

The number of RunPod workers is configurable — at the system level via an environment variable and per-user via Settings → API Keys, where each user can also store their own RunPod API key and endpoint ID. The cap is set conservatively by default to avoid runaway parallelism, but can be raised for large backlogs.

---

## Choosing Your Provider — and What Happens When It's Not Available

With five providers integrated, a natural question arises: which one runs for a given request? The app handles this at two levels.

### Manual selection and ordered fallback

For straightforward use, you simply pick a provider per conversation — Claude, Gemini, DeepSeek, OpenAI, or local Ollama. But manual selection alone isn't robust. APIs have rate limits, quota exhaustion, and occasional outages. So each user can configure an **ordered fallback chain** for hosted providers: something like `[claude, gemini, deepseek, openai]`. When the primary provider is unavailable, the system works down the list until it finds one that responds.

There are actually two separate orderings you can configure — an **auto order** (used when Auto routing selects a hosted provider) and a **failover order** (used when a manually selected provider fails mid-session). Failover can also be disabled entirely if you want hard failures rather than silent switching.

One additional nuance: if you've manually chosen a provider in a recent conversation, Auto mode remembers that choice and tries it first before consulting the configured order. The idea is that your most recent explicit preference is probably still your preference.

---

## The Query Classifier — Using AI to Decide Which AI to Use

The most technically interesting piece of the provider system is the **Auto routing mode**. Instead of the user picking a provider, Auto mode runs a lightweight classifier first that analyses the incoming request and decides: can this be handled by the local small model, or does it genuinely need a full-featured hosted LLM?

The classifier sends the user's prompt — along with metadata about how many tools are available, how many reference documents exist, and whether a subject profile has been built — to a compact AI model and asks for a structured JSON verdict:

```json
{
  "route": "local",
  "reason": "Simple date lookup — one tool call required, no reasoning needed",
  "confidence": 0.92,
  "needs_reference_documents": false,
  "needs_user_profile": false
}
```

**Route local** when the request needs just one or two simple tool calls, a factual lookup, or a brief answer with minimal inference. **Route hosted** when the request needs multi-step reasoning, synthesising across many records, rich prose, persona-heavy responses, or anything emotionally nuanced.

The two boolean flags — `needs_reference_documents` and `needs_user_profile` — are a particularly useful optimisation. Reference documents and psychological profiles can be large. If the classifier determines a question doesn't require them, they're omitted from the downstream system prompt entirely, reducing token cost regardless of which provider handles the full request.

### The classifier itself defaults to local AI

Here's the detail I find most interesting: the classifier defaults to running on the **local Ollama model**. So a cheap, private, on-device AI decides whether a given question is worth sending to a cloud provider at all. The overhead of the classification step is small, and the savings on simple queries — current time, a message count, a single name lookup — add up quickly.

The classifier has its own fallback chain too. If local AI isn't running, it tries the configured hosted providers in order. And if the classifier itself fails or returns unparseable output, the system defaults conservatively to routing the request to a hosted provider with full context — never silently dropping the user's question.

---

## The Full Picture

Here's how each use maps to the provider types:

| Use | Hosted LLM | Local AI | Local Embedding | Hosted On Demand |
|---|:---:|:---:|:---:|:---:|
| Conversational chat (archivist) | ✓ | ✓ | | |
| Voice personalities | ✓ | ✓ | | |
| Two-voice conversation | ✓ | ✓ | | |
| Structured interviews | ✓ | ✓ | | |
| Memory companion (Pam Bot) | ✓ | ✓ | | |
| Identity profile builder | ✓ | ✓ | | |
| Random question generation | ✓ | ✓ | | |
| Summarisation | ✓ | ✓ | | |
| Writing style analysis | ✓ | ✓ | | |
| Web search (Tavily) | ✓ | | | |
| Query classification (Auto routing) | ✓ ¹ | ✓ ² | | |
| Image classification & tagging | | ✓ ² | | ✓ |
| Content vectorisation (text & image tags) | | | ✓ | |
| Semantic similarity search | | | ✓ | |

*¹ Fallback when Local AI is unavailable · ² Primary*

A few things stand out in this view. First, local AI participates in almost everything — it's not a reduced-capability alternative but a genuine first-class option for most tasks. Second, the hosted providers and local AI are largely interchangeable for conversational uses; the choice is one of cost, privacy, and quality trade-off rather than capability. Third, local embedding sits in its own lane: it's not interchangeable with the others and handles a fundamentally different job. And fourth, hosted on-demand (RunPod) appears only where the problem is throughput rather than capability — the model doing the work is the same Gemma4 that runs locally, just replicated across parallel serverless workers.

---

## AI as Co-Developer: How the Coding Experience Changed

The product uses AI extensively. But so did the process of building it — and that story is almost as interesting as the product itself.

### The evolution: from autocomplete to autonomous agent

When the project started in Python, AI assistance meant inline code completion. Useful, but fundamentally reactive: you wrote, it suggested the next token. The cognitive load of design, architecture, and cross-file consistency remained entirely with me.

As the project grew and migrated to Go, the tooling shifted to chat-based assistance. I could describe a problem, paste a snippet, and get a working solution back. This was a step change in productivity for self-contained tasks — writing a new repository method, debugging a tricky SQLite dialect issue, translating a data structure. But the AI still had no awareness of the broader codebase. Every conversation started cold.

The transformation came with agentic coding tools that could read the entire codebase, understand the existing patterns, and make multi-file changes autonomously. At that point the nature of the work changed. Instead of writing code assisted by AI, I was increasingly *directing* AI that was writing code. The shift is subtle but profound: the bottleneck moved from typing to thinking — from implementation to specification.

By the later stages of the project, it was entirely normal to describe a new feature at a high level — "add a background job that pre-computes embeddings for any message that doesn't have one yet, scoped to the current user, with SSE progress streaming" — and have a working, idiomatic implementation across four or five files returned without writing a line of it myself. The AI understood the existing patterns well enough to follow them: the repository layer conventions, the user-scoping rules, the import goroutine safety requirements.

### What AI does well in this codebase

**Idiomatic Go.** The language's explicitness suits AI well. Strong typing, clear interfaces, and predictable patterns mean AI rarely produces subtly wrong code that compiles but misbehaves. Error handling, context threading, and goroutine patterns were all handled correctly once the conventions were established in the codebase.

**Boilerplate at scale.** The repository layer has around thirty files following near-identical patterns: a struct, a constructor, a handful of methods each doing parameterised SQL with user-scoping. Generating a new repository from a description of what it should do took seconds. Getting it *right* without AI would have taken an afternoon.

**Refactoring with a defined target.** When I decided to change how user IDs were threaded through background goroutines — capturing `uid` before launch and passing it via `context.WithValue` rather than inheriting the HTTP context — AI applied the pattern consistently across all existing import handlers. A change I'd been dreading took one pass.

**SQL.** Every query in the codebase uses parameterised placeholders and explicit user-scoping. AI was diligent about this — it never produced a query that used string concatenation or forgot the `AND user_id = $N` filter. Whether that's because the surrounding code reinforced the pattern or because the models genuinely understand the security implication, the result was reliable.

**Test generation.** Given a function and its intended behaviour, AI produced useful unit tests quickly, including edge cases I hadn't explicitly specified. Coverage for the auto-routing classifier, the provider fallback logic, and the SQLite migration helpers all came from AI-generated test suites that I reviewed rather than wrote.

### Where it still needs direction

**Vanilla JavaScript across a large file.** The frontend is ~22 modules of plain JavaScript with no framework. AI handles individual modules well but can lose coherence in the 8,000-line CSS file or when a change needs to be consistent across many JS files simultaneously. Explicit instruction about which patterns to follow — the revealing module pattern, the DOM element cache in `foundation.js` — was necessary on every session.

**CGO build complexity.** The `sqlite3` CGO dependency, the `cgo-compat` shim, the Makefile `CGO_CFLAGS` configuration — AI understood the *concepts* but consistently underestimated how brittle CGO toolchain configuration is on Windows. Debugging `cgo.exe: exit status 2` errors required human intuition about PATH ordering and DLL resolution that no AI recovered correctly on the first attempt.

**Architectural decisions.** AI is excellent at implementing a decision. It is less reliable at making one. Choosing between embedding vectors in SQLite vs. a separate vector store, deciding whether the dual-Ollama architecture was worth the complexity, designing the auto-routing classifier's fallback chain — these required human judgement about trade-offs that AI could inform but not resolve.

### Which models were better at what

Not all models performed equally across the different coding tasks in this project.

**Claude** was consistently the strongest for complex, multi-file changes. Its ability to hold the context of the entire codebase, follow existing conventions without being told, and reason about the *consequences* of a change before making it made it the default choice for anything architectural. It was also noticeably more careful about security-relevant patterns — user scoping, parameterised SQL, cookie flags — than the alternatives.

**Gemini** handled certain well-defined, self-contained tasks very well, particularly when the task involved transforming or generating structured content: seed data files, JSON schemas, configuration templates. Its larger context window was occasionally useful for passing in large swathes of existing code as reference without truncation.

**DeepSeek** was a practical surprise for routine backend work — repository methods, handler boilerplate, migration helpers. The quality was close to Claude for well-defined tasks at a fraction of the cost, which made it a sensible choice when the task was clear and the stakes of getting it subtly wrong were low.

**Local models (Gemma4 via Ollama)** were not used for coding assistance in any serious capacity. Their role in this project is as runtime AI, not development AI. The gap in capability for complex, context-heavy coding tasks remains significant.

The most useful mental model I developed was roughly: use Claude when the task requires understanding the *shape* of the codebase and making judgements; use a capable hosted model for well-specified implementation work; don't expect any model to substitute for architectural thinking.

---

## What I'd Do Differently

A few things I learned the hard way:

**Standardise on one tool-calling format first.** The differences between Claude's `tool_use` and OpenAI's `tool_calls` are small but require separate code paths throughout. If I were starting again I'd build a clean internal abstraction before adding the second provider.

**Treat the embedding pipeline as infrastructure, not a feature.** Running embeddings asynchronously in the background is the right call, but it took longer to get right than I expected. The background job scheduler that now handles it should have been built first.

**The system prompt is load-bearing.** Getting pronoun substitution, subject configuration, voice instructions, and reference documents all injected correctly — in the right order, at the right size — turned out to be one of the more intricate parts of the project. It deserves its own abstraction layer.

---

## Closing Thought

The most interesting thing about building this isn't the AI itself — it's how quickly the question changes from "can the AI answer this?" to "which role should the AI be playing right now, and with whose voice?"

A personal archive is a fundamentally human artefact. The AI's job isn't to replace that humanity but to make it navigable. Getting that right is an ongoing design problem, not an engineering one.

---

*Digital Museum is a personal project. If you're building something similar and want to compare notes, feel free to connect.*
