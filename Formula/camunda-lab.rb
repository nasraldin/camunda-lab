# Homebrew formula for the nasraldin/tools tap (repo: nasraldin/homebrew-tools).
#
# Formula name: camunda-lab
# Installed CLI binary: camunda
#
# Source of truth in this repo. On GitHub Release, CI runs
# scripts/publish-homebrew.sh to push url + sha256 into the tap.
#
# Users:
#   brew tap nasraldin/tools
#   brew install camunda-lab
#
class CamundaLab < Formula
  desc "Unofficial local Camunda 8 Docker lab CLI"
  homepage "https://github.com/nasraldin/camunda-lab"
  url "https://github.com/nasraldin/camunda-lab/archive/refs/tags/v0.7.0.tar.gz"
  sha256 "3bc32d064b12e8554153556a3e7f6e9d0af29334a4ef6f1c2a84b4b1bbb6f0e0"
  license "MIT"

  depends_on "go" => :build
  depends_on "docker"

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"camunda"), "./cmd/camunda"
  end

  def post_install
    # Start Lab UI in the background so browser-only users can install from Setup.
    system bin/"camunda", "ui", "--no-open", out: File::NULL, err: File::NULL
  end

  def caveats
    <<~EOS
      The CLI is `camunda` (formula name is camunda-lab).

      You need Docker Compose v2 (`docker compose version`).

      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
      Lab UI is running — open in your browser:

        http://localhost:9090

      Install and manage Camunda from the UI (no terminal required).
      Use the "Get started" page to install your first lab.
      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

      Prefer the CLI?

        camunda install --version 8.9 --profile light --yes
        camunda wait
        camunda urls

      Stop the UI: camunda ui --stop
      UI logs: camunda ui logs -f
      Docs: https://nasraldin.github.io/camunda-lab/
    EOS
  end

  test do
    assert_match "camunda-lab", shell_output("#{bin}/camunda version")
  end
end
