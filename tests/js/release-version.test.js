/** @jest-environment node */

const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  calculateVersion,
  computeReleaseVersion,
  parseVersion,
} = require("../../.github/scripts/release-version.cjs");

const temporaryRepositories = [];

function git(repo, ...args) {
  return execFileSync("git", args, {
    cwd: repo,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function commit(repo, message) {
  git(repo, "add", ".");
  git(
    repo,
    "-c",
    "user.name=Release Test",
    "-c",
    "user.email=release-test@example.invalid",
    "commit",
    "-m",
    message,
  );
}

function createRepository(version) {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "servicarr-release-version-"),
  );
  temporaryRepositories.push(repo);
  git(repo, "init", "--quiet");

  const versionFile = path.join(
    repo,
    "app",
    "internal",
    "buildinfo",
    "VERSION",
  );
  fs.mkdirSync(path.dirname(versionFile), { recursive: true });
  fs.writeFileSync(versionFile, `${version}\n`, "utf8");
  commit(repo, `set version ${version}`);
  return repo;
}

afterEach(() => {
  while (temporaryRepositories.length > 0) {
    fs.rmSync(temporaryRepositories.pop(), { recursive: true, force: true });
  }
});

describe("release version calculation", () => {
  test("verifies the published version with its embedded release commit", () => {
    const workflow = fs.readFileSync(
      path.join(__dirname, "../../.github/workflows/release.yml"),
      "utf8",
    );

    expect(workflow).toContain(
      "RELEASE_SHA: ${{ steps.version.outputs.commit }}",
    );
    expect(workflow).toContain(
      'EXPECTED_VERSION="Servicarr v${RELEASE_VERSION} (${RELEASE_SHA:0:12})"',
    );
    expect(workflow).toContain(
      'test "$ACTUAL_VERSION" = "$EXPECTED_VERSION"',
    );
  });

  test("requires a strict semantic base version", () => {
    expect(parseVersion("1.2.3")).toMatchObject({
      major: 1,
      minor: 2,
      patch: 3,
    });
    expect(() => parseVersion("v1.2.3")).toThrow(/MAJOR\.MINOR\.PATCH/);
    expect(() => parseVersion("1.2")).toThrow(/MAJOR\.MINOR\.PATCH/);
    expect(() => calculateVersion("1.2.3", -1)).toThrow(/offset/);
  });

  test("increments once per first-parent commit and resets on an explicit version bump", () => {
    const repo = createRepository("1.0.1");

    expect(computeReleaseVersion(repo)).toMatchObject({
      baseVersion: "1.0.1",
      version: "1.0.1",
      offset: 0,
      tag: "v1.0.1",
    });

    fs.writeFileSync(path.join(repo, "README.md"), "next change\n", "utf8");
    commit(repo, "add a change");
    expect(computeReleaseVersion(repo)).toMatchObject({
      version: "1.0.2",
      offset: 1,
    });

    const versionFile = path.join(
      repo,
      "app",
      "internal",
      "buildinfo",
      "VERSION",
    );
    fs.writeFileSync(versionFile, "2.0.0\n", "utf8");
    commit(repo, "start version 2");
    expect(computeReleaseVersion(repo)).toMatchObject({
      baseVersion: "2.0.0",
      version: "2.0.0",
      offset: 0,
      tag: "v2.0.0",
    });
  });

  test("treats a regular merge that changes the version as the new baseline", () => {
    const repo = createRepository("1.0.1");
    const mainBranch = git(repo, "branch", "--show-current");

    git(repo, "checkout", "--quiet", "-b", "release-line");
    const versionFile = path.join(
      repo,
      "app",
      "internal",
      "buildinfo",
      "VERSION",
    );
    fs.writeFileSync(versionFile, "1.1.0\n", "utf8");
    commit(repo, "start version 1.1");

    git(repo, "checkout", "--quiet", mainBranch);
    git(
      repo,
      "-c",
      "user.name=Release Test",
      "-c",
      "user.email=release-test@example.invalid",
      "merge",
      "--no-ff",
      "release-line",
      "-m",
      "merge release line",
    );

    expect(computeReleaseVersion(repo)).toMatchObject({
      baseVersion: "1.1.0",
      version: "1.1.0",
      offset: 0,
      tag: "v1.1.0",
    });
  });
});
