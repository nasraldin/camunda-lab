# Homebrew formula stub — update url/sha256 when cutting a release.
# Binary name: camunda  |  Formula name: camunda-lab
class CamundaLab < Formula
  desc "Unofficial local Camunda 8 Docker lab CLI"
  homepage "https://github.com/nasraldin/camunda-lab"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/nasraldin/camunda-lab/releases/download/v0.1.0/camunda-lab_0.1.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_intel do
      url "https://github.com/nasraldin/camunda-lab/releases/download/v0.1.0/camunda-lab_0.1.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/nasraldin/camunda-lab/releases/download/v0.1.0/camunda-lab_0.1.0_linux_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_intel do
      url "https://github.com/nasraldin/camunda-lab/releases/download/v0.1.0/camunda-lab_0.1.0_linux_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  def install
    bin.install "camunda"
  end

  test do
    assert_match "camunda-lab", shell_output("#{bin}/camunda version")
  end
end
