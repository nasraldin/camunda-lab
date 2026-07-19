const KEY = 'camunda-lab-project-dir'

export function getProjectDir(): string {
  try {
    return localStorage.getItem(KEY) || ''
  } catch {
    return ''
  }
}

export function setProjectDir(dir: string): void {
  try {
    localStorage.setItem(KEY, dir)
  } catch {
    /* ignore */
  }
}
