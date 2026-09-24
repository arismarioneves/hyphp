import { useCallback, useEffect, useState } from 'react'
import { ProjectsService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { Project } from './types'

export function useProjects() {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const list = await ProjectsService.List()
    setProjects(list ?? [])
    setLoading(false)
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEvent<Project[]>(EVENTS.projectChanged, (list) => {
    setProjects(list ?? [])
    setLoading(false)
  })

  return { projects, loading, refresh }
}
