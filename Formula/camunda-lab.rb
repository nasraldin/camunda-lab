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
  url "https://github.com/nasraldin/camunda-lab/archive/refs/tags/v0.1.1.tar.gz"
  sha256 "17512f4f6e3fc714af0ddf20a868c507ac76106c8284db794a60563204559e49"
  license "MIT"

  depends_on "go" => :build
  depends_on "docker"

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"camunda"), "./cmd/camunda"
  end

  def caveats
    <<~EOS
      The CLI is `camunda` (formula name is camunda-lab).

      You need Docker Compose v2 (`docker compose version`).

      After install:

        camunda install --version 8.8 --profile light --yes
        camunda wait
        camunda urls

      Docs: https://nasraldin.github.io/camunda-lab/
    EOS
  end

  test do
    assert_match "camunda-lab", shell_output("#{bin}/camunda version")
  end
end
