const { execFileSync } = require("node:child_process");
const path = require("node:path");

const VERSION_PATH = "app/internal/buildinfo/VERSION";

function runGit(repoRoot, args) {
  return execFileSync("git", args, {
    cwd: repoRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function parseVersion(value) {
  const normalized = String(value).trim();
  const match = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(normalized);
  if (!match) {
    throw new Error(
      `Invalid release base version "${normalized}"; expected MAJOR.MINOR.PATCH`,
    );
  }

  const numbers = match.slice(1).map(Number);
  if (numbers.some((part) => !Number.isSafeInteger(part))) {
    throw new Error(
      `Release base version "${normalized}" exceeds JavaScript's safe integer range`,
    );
  }

  return {
    version: normalized,
    major: numbers[0],
    minor: numbers[1],
    patch: numbers[2],
  };
}

function calculateVersion(baseVersion, offset) {
  const base = parseVersion(baseVersion);
  if (!Number.isSafeInteger(offset) || offset < 0) {
    throw new Error(`Invalid first-parent release offset "${offset}"`);
  }

  const patch = base.patch + offset;
  if (!Number.isSafeInteger(patch)) {
    throw new Error(
      "Calculated release patch exceeds JavaScript's safe integer range",
    );
  }

  return {
    ...base,
    baseVersion: base.version,
    version: `${base.major}.${base.minor}.${patch}`,
    majorMinor: `${base.major}.${base.minor}`,
    offset,
  };
}

function computeReleaseVersion(
  repoRoot = process.cwd(),
  target = "HEAD",
  git = runGit,
) {
  const root = path.resolve(repoRoot);
  const commit = git(root, ["rev-parse", "--verify", `${target}^{commit}`]);
  const baseVersion = git(root, ["show", `${commit}:${VERSION_PATH}`]);
  const baselineCommit = git(root, [
    "log",
    "--first-parent",
    "-1",
    "--format=%H",
    commit,
    "--",
    VERSION_PATH,
  ]);

  if (!baselineCommit) {
    throw new Error(
      `Could not find the commit that established ${VERSION_PATH}`,
    );
  }

  const offsetText = git(root, [
    "rev-list",
    "--first-parent",
    "--count",
    `${baselineCommit}..${commit}`,
  ]);
  const offset = Number(offsetText);
  const release = calculateVersion(baseVersion, offset);

  return {
    ...release,
    commit,
    baselineCommit,
    tag: `v${release.version}`,
  };
}

function formatGitHubOutput(release) {
  return [
    `version=${release.version}`,
    `tag=${release.tag}`,
    `major=${release.major}`,
    `minor=${release.minor}`,
    `major_minor=${release.majorMinor}`,
    `base_version=${release.baseVersion}`,
    `offset=${release.offset}`,
    `commit=${release.commit}`,
    `baseline_commit=${release.baselineCommit}`,
  ].join("\n");
}

if (require.main === module) {
  try {
    const release = computeReleaseVersion(
      process.cwd(),
      process.argv[2] || "HEAD",
    );
    process.stdout.write(`${formatGitHubOutput(release)}\n`);
  } catch (error) {
    process.stderr.write(`release-version: ${error.message}\n`);
    process.exitCode = 1;
  }
}

module.exports = {
  VERSION_PATH,
  calculateVersion,
  computeReleaseVersion,
  formatGitHubOutput,
  parseVersion,
};
