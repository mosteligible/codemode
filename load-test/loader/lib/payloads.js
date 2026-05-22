function integerFromEnv(name, fallback) {
	const parsed = Number.parseInt(__ENV[name] ?? '', 10);
	return Number.isFinite(parsed) ? parsed : fallback;
}

function stringFromEnv(name, fallback) {
	const value = __ENV[name];
	if (typeof value !== 'string') {
		return fallback;
	}

	const trimmed = value.trim();
	return trimmed === '' ? fallback : trimmed;
}

function shortCommand() {
	return "printf 'short-ok\\n'";
}

function exploreCommand() {
	const files = integerFromEnv('EXPLORE_FILE_COUNT', 48);
	const dirs = integerFromEnv('EXPLORE_DIR_COUNT', 6);
	const reads = integerFromEnv('EXPLORE_READ_COUNT', 8);
	return [
		"python - <<'PY'",
		'import hashlib',
		'import pathlib',
		'import tempfile',
		`total_files = ${files}`,
		`dir_count = max(1, ${dirs})`,
		`read_count = max(1, ${reads})`,
		'with tempfile.TemporaryDirectory() as root:',
		'    root_path = pathlib.Path(root)',
		'    for index in range(total_files):',
		'        branch = root_path / f"dir_{index % dir_count:02d}" / f"group_{(index // dir_count) % 3:02d}"',
		'        branch.mkdir(parents=True, exist_ok=True)',
		'        file_path = branch / f"entry_{index:03d}.txt"',
		'        file_path.write_text(f"file={index}\\ndir={index % dir_count}\\nvalue={index * 7}\\n")',
		'    discovered = sorted(root_path.rglob("*.txt"))',
		'    digest = hashlib.sha256()',
		'    for path in discovered[:read_count]:',
		'        digest.update(path.read_bytes())',
		'    unique_dirs = len({path.parent for path in discovered})',
		'    print(f"explore-ok:{len(discovered)}:{unique_dirs}:{digest.hexdigest()[:12]}")',
		'PY',
	].join('\n');
}

function jsonCommand() {
	const records = integerFromEnv('JSON_RECORD_COUNT', 300);
	return [
		"python - <<'PY'",
		'import json',
		'import pathlib',
		'import tempfile',
		`record_count = ${records}`,
		'with tempfile.TemporaryDirectory() as root:',
		'    root_path = pathlib.Path(root)',
		'    payload_path = root_path / "payload.json"',
		'    payload = [',
		'        {',
		'            "id": index,',
		'            "bucket": index % 7,',
		'            "name": f"item-{index}",',
		'            "meta": {"score": index * 3, "enabled": index % 2 == 0},',
		'        }',
		'        for index in range(record_count)',
		'    ]',
		'    payload_path.write_text(json.dumps(payload))',
		'    parsed = json.loads(payload_path.read_text())',
		'    enabled_total = sum(item["meta"]["score"] for item in parsed if item["meta"]["enabled"])',
		'    bucket_counts = {}',
		'    for item in parsed:',
		'        bucket_counts[item["bucket"]] = bucket_counts.get(item["bucket"], 0) + 1',
		'    print(f"json-ok:{len(parsed)}:{enabled_total}:{bucket_counts.get(0, 0)}")',
		'PY',
	].join('\n');
}

function cpuCommand() {
	const iterations = integerFromEnv('CPU_PYTHON_ITERATIONS', 750000);
	return [
		"python - <<'PY'",
		'import math',
		'acc = 0.0',
		`for i in range(${iterations}):`,
		'    acc += math.sqrt((i % 1000) + 1)',
		"print(f'cpu-ok:{acc:.2f}')",
		'PY',
	].join('\n');
}

function networkCommand() {
	const url = stringFromEnv('NETWORK_URL', 'https://example.com');
	const timeoutSeconds = integerFromEnv('NETWORK_TIMEOUT_SECONDS', 5);
	return [
		"python - <<'PY'",
		'import hashlib',
		'import urllib.request',
		`url = ${JSON.stringify(url)}`,
		`timeout_seconds = ${timeoutSeconds}`,
		'with urllib.request.urlopen(url, timeout=timeout_seconds) as response:',
		'    body = response.read(4096)',
		'    digest = hashlib.sha256(body).hexdigest()[:12]',
		'    print(f"network-ok:{response.status}:{len(body)}:{digest}")',
		'PY',
	].join('\n');
}

export const payloadProfiles = {
	short: {
		name: 'short',
		language: 'bash',
		code: shortCommand(),
		expectText: 'short-ok',
	},
	explore: {
		name: 'explore',
		language: 'bash',
		code: exploreCommand(),
		expectText: 'explore-ok:',
	},
	json: {
		name: 'json',
		language: 'bash',
		code: jsonCommand(),
		expectText: 'json-ok:',
	},
	cpu: {
		name: 'cpu',
		language: 'bash',
		code: cpuCommand(),
		expectText: 'cpu-ok:',
	},
	network: {
		name: 'network',
		language: 'bash',
		code: networkCommand(),
		expectText: 'network-ok:',
	},
};

function buildWeightedMix() {
	const shortWeight = integerFromEnv('MIX_SHORT_WEIGHT', 70);
	const exploreWeight = integerFromEnv('MIX_EXPLORE_WEIGHT', 20);
	const jsonWeight = integerFromEnv('MIX_JSON_WEIGHT', 8);
	const cpuWeight = integerFromEnv('MIX_CPU_WEIGHT', 2);
	const networkWeight = integerFromEnv('MIX_NETWORK_WEIGHT', 0);
	const mix = [];

	for (let index = 0; index < shortWeight; index += 1) {
		mix.push(payloadProfiles.short);
	}
	for (let index = 0; index < exploreWeight; index += 1) {
		mix.push(payloadProfiles.explore);
	}
	for (let index = 0; index < jsonWeight; index += 1) {
		mix.push(payloadProfiles.json);
	}
	for (let index = 0; index < cpuWeight; index += 1) {
		mix.push(payloadProfiles.cpu);
	}
	for (let index = 0; index < networkWeight; index += 1) {
		mix.push(payloadProfiles.network);
	}

	if (mix.length === 0) {
		return [payloadProfiles.short];
	}

	return mix;
}

export function getPayloadProfile(name) {
	const payload = payloadProfiles[name];
	if (!payload) {
		throw new Error(`unknown payload profile: ${name}`);
	}
	return payload;
}

export function pickMixedPayload(iterationInTest) {
	const mix = buildWeightedMix();
	return mix[iterationInTest % mix.length];
}

export function buildBoundedLongPayload(iterationInTest) {
	const minSeconds = integerFromEnv('LONG_FORM_MIN_SECONDS', 5);
	const maxSeconds = integerFromEnv('LONG_FORM_MAX_SECONDS', 25);
	const lowerBound = Math.min(minSeconds, maxSeconds);
	const upperBound = Math.max(minSeconds, maxSeconds);
	const span = upperBound - lowerBound + 1;
	const durationSeconds = lowerBound + (iterationInTest % span);

	return {
		name: 'bounded-long',
		language: 'bash',
		code: [
			"python - <<'PY'",
			'import math',
			'import time',
			`deadline = time.time() + ${durationSeconds}`,
			'acc = 0.0',
			'i = 0',
			'while time.time() < deadline:',
			'    acc += math.sqrt((i % 1000) + 1)',
			'    i += 1',
			`print(f"bounded-long-ok:${durationSeconds}:{i}:{acc:.2f}")`,
			'PY',
		].join('\n'),
		expectText: 'bounded-long-ok:',
		durationSeconds,
	};
}
