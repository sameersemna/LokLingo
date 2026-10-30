import assert from "node:assert/strict"
import { randomBytes } from "node:crypto"
import { spawnSync } from "node:child_process"
import { createServer } from "node:net"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const project = `loklingo-prod-check-${randomBytes(6).toString("hex")}`
const writeToken = randomBytes(32).toString("hex")
const internalToken = randomBytes(32).toString("hex")
const env = { ...process.env, WRITE_API_TOKEN: writeToken, INTERNAL_TOKEN: internalToken }

function command(program, args, options = {}) {
  const result = spawnSync(program, args, { cwd: root, env, encoding: "utf8", ...options })
  if (result.status !== 0) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
    throw new Error(`${program} failed with status ${result.status}`)
  }
  return result.stdout ?? ""
}

async function unusedPort() {
  const server = createServer()
  await new Promise((resolveListen, reject) => {
    server.once("error", reject)
    server.listen(0, "127.0.0.1", resolveListen)
  })
  const port = server.address().port
  await new Promise((resolveClose, reject) => server.close(error => error ? reject(error) : resolveClose()))
  return port
}

async function main() {
  const config = JSON.parse(command("docker", ["compose", "--profile", "production", "-f", "docker-compose.yml", "-f", "docker-compose.prod.yml", "config", "--format", "json"]))
  const originalProject = config.name
  const modelSource = config.volumes.ocr_models.name
  const ports = { frontend: await unusedPort(), backend: await unusedPort(), ocr: await unusedPort() }
  const serviceNames = ["loklingo-backend", "loklingo-frontend", "loklingo-ocr"]
  config.name = project
  config.services = Object.fromEntries(serviceNames.map(name => [name, config.services[name]]))
  for (const [name, service] of Object.entries(config.services)) {
    delete service.container_name
    delete service.build
    delete service.profiles
    delete service.depends_on
    service.image = service.image ?? `${originalProject}-${name}`
    const role = name.replace("loklingo-", "")
    service.ports = [{ target: role === "ocr" ? 8000 : 8080, published: String(ports[role]), host_ip: "127.0.0.1", protocol: "tcp" }]
  }
  for (const network of Object.values(config.networks)) delete network.name
  delete config.volumes.ollama_data
  for (const [name, volume] of Object.entries(config.volumes)) volume.name = `${project}_${name}`
  const backend = config.services["loklingo-backend"]
  backend.environment.REDIS_URL = "redis://validation-redis:6379/0"
  backend.environment.OCR_SERVICE_URL = "http://loklingo-ocr:8000"
  backend.environment.POSTGRES_DSN = ""
  backend.environment.LITELLM_BASE_URL = "http://validation-provider:4010"
  backend.environment.LITELLM_MODEL = "mock-model"
  backend.environment.LITELLM_API_KEY = randomBytes(32).toString("hex")
  backend.environment.OLLAMA_BASE_URL = ""
  backend.environment.OLLAMA_MODEL = ""
  backend.environment.OPENAI_COMPAT_BASE_URL = ""
  backend.environment.OPENAI_COMPAT_MODEL = ""
  config.services["loklingo-ocr"].environment.OCR_CORRECTION_ENABLED = "false"
  config.services["validation-redis"] = { image: "redis:7-alpine", networks: ["loklingo-net"] }
  config.services["validation-provider"] = {
    image: "python:3.11-slim",
    command: ["python", "/validation/mock_litellm.py"],
    volumes: [{ type: "bind", source: resolve(root, "guide/mock_litellm.py"), target: "/validation/mock_litellm.py", read_only: true }],
    networks: ["loklingo-net"],
  }
  const input = JSON.stringify(config)
  const compose = args => command("docker", ["compose", "--project-name", project, "-f", "-", ...args], { input })
  compose(["config", "--quiet"])
  if (process.argv.includes("--check")) {
    console.log("Isolated production Compose configuration validated; no containers started.")
    return
  }
  console.log("Production validation uses an isolated Redis queue, temporary credentials, and the explicit mock translation provider. Postgres analytics is tested separately in development.")
  try {
    const modelTarget = config.volumes.ocr_models.name
    command("docker", ["volume", "create", "--label", `com.docker.compose.project=${project}`, "--label", "com.docker.compose.volume=ocr_models", modelTarget])
    command("docker", ["run", "--rm", "--user", "0", "--entrypoint", "sh", "-v", `${modelSource}:/source:ro`, "-v", `${modelTarget}:/target`, config.services["loklingo-ocr"].image, "-c", "cp -a /source/. /target/"])
    process.stdout.write(compose(["up", "-d", "--wait", "--wait-timeout", "180"]))
    const backendURL = `http://127.0.0.1:${ports.backend}`
    const frontendURL = `http://127.0.0.1:${ports.frontend}`
    assert.equal((await fetch(`${backendURL}/ready`)).status, 200)
    const denied = await fetch(`${backendURL}/api/v1/jobs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text: "hello world", source: "en", target: "de" }) })
    assert.equal(denied.status, 401, "production write endpoints must reject missing tokens")
    assert.equal((await fetch(`${backendURL}/api/v1/metrics/providers`)).status, 401)
    assert.equal((await fetch(`${backendURL}/api/v1/metrics/providers`, { headers: { "X-Internal-Token": internalToken } })).status, 200)
    for (const name of serviceNames) {
      const id = compose(["ps", "-q", name]).trim()
      const [container] = JSON.parse(command("docker", ["inspect", id]))
      assert.notEqual(container.Config.User, "root")
      assert.notEqual(container.Config.User, "")
      assert(container.HostConfig.CapDrop.includes("ALL"))
      assert(container.HostConfig.SecurityOpt.some(option => option.startsWith("no-new-privileges")))
      assert(container.HostConfig.Memory > 0)
      assert(container.HostConfig.NanoCpus > 0)
      if (name !== "loklingo-ocr") assert.equal(container.HostConfig.ReadonlyRootfs, true)
    }
    console.log("Production readiness, token enforcement, non-root users, capability limits, read-only filesystems, and resource limits verified.")
    command("npm", ["test", "--", "tests/image-pdf-smoke.spec.ts", "tests/translation-flow.spec.ts", "--grep", "image job completes|valid PDF uploads|PDF job endpoint rejects|exports a valid PDF", "--reporter=list"], {
      cwd: resolve(root, "e2e"),
      env: { ...env, LOKLINGO_E2E_BASE_URL: frontendURL, LOKLINGO_BASE_URL: backendURL, LOKLINGO_E2E_ANALYTICS: "0", PLAYWRIGHT_HTML_OPEN: "never" },
      stdio: "inherit",
    })
    console.log("Isolated production runtime validation passed.")
  } catch (error) {
    process.stdout.write(compose(["logs", "--no-color", "--tail", "40"]))
    throw error
  } finally {
    process.stdout.write(compose(["down", "--volumes", "--remove-orphans"]))
  }
}

main().catch(error => {
  console.error(error.message)
  process.exitCode = 1
})