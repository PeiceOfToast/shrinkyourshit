// SPDX-License-Identifier: AGPL-3.0-only

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import type { FlowGraph, Profile, RuleTier, StepTypeInfo } from './api'
import FlowTreePage from './FlowTreePage'
import ProfileEditorPage, { ProfilesSection } from './ProfilesPage'
import {
  installFetchMock,
  type FetchMock,
  type MockRoute,
} from './test/fetchMock'
import { ToastProvider } from './toast'

const PALETTE: StepTypeInfo[] = [
  {
    type: 'probe',
    label: 'Check the file',
    description: '',
    condition: false,
    params: [],
  },
  {
    type: 'if_bitrate_above',
    label: 'Only when bitrate is above',
    description: '',
    condition: true,
    params: [
      {
        name: 'mbps',
        label: 'megabits per second',
        kind: 'number',
        min: 0.1,
        required: true,
        default: 8,
      },
    ],
  },
  {
    type: 'skip_if_smaller',
    label: 'Keep the original if smaller',
    description: '',
    condition: false,
    params: [],
  },
  {
    type: 'transcode_video',
    label: 'Encode the video',
    description: '',
    condition: false,
    params: [],
  },
]

function graphOf(ids: string[]): FlowGraph {
  return {
    schema: 1,
    nodes: ids.map((id) => ({ id, type: id })),
    edges: ids.slice(1).map((to, i) => ({ from: ids[i], to })),
  }
}

/** A graph with a probe and an encode: fine at every tier. */
const BASIC_GRAPH = graphOf(['probe', 'transcode_video'])
/** Preview result for the Basic form: what the sliders compile to. */
const PREVIEW_GRAPH = graphOf(['probe', 'transcode_video', 'skip_if_smaller'])

function profileDoc(over: Partial<Profile> = {}): Profile {
  return {
    id: 'p1',
    name: 'Films',
    description: 'Shared film rules',
    tier: 'basic',
    result_policy: 'replace_if_smaller',
    graph: BASIC_GRAPH,
    used_by: 0,
    used_by_names: [],
    unrepresentable: [],
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-05T10:00:00Z',
    ...over,
  }
}

/** Routes every profile editor needs: the one profile, the palette and previews. */
function editorRoutes(
  profile: Profile,
  lockFor?: (tier: string) => Profile['unrepresentable'],
): MockRoute[] {
  return [
    {
      method: 'GET',
      match: /\/api\/v1\/profiles\/p1$/,
      bodyFn: ({ url }) => {
        const tier = url.searchParams.get('tier')
        if (tier && lockFor) {
          return { ...profile, unrepresentable: lockFor(tier) }
        }
        return profile
      },
    },
    {
      method: 'PUT',
      match: /\/api\/v1\/profiles\/p1$/,
      bodyFn: ({ body }) => ({ ...profile, ...(body as object) }),
    },
    {
      method: 'POST',
      match: /\/api\/v1\/flows\/preview$/,
      body: { graph: PREVIEW_GRAPH, steps: [] },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/flows\/step-types$/,
      body: { items: PALETTE, total: PALETTE.length },
    },
    {
      method: 'GET',
      match: /\/api\/v1\/plugins$/,
      body: { items: [], total: 0 },
    },
  ]
}

function renderWith(path: string, defaultTier: RuleTier = 'basic') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route
              path="/encode"
              element={<ProfilesSection defaultTier={defaultTier} />}
            />
            <Route
              path="/encode/profiles/:id"
              element={<ProfileEditorPage />}
            />
            <Route
              path="/encode/profiles/:id/tree"
              element={<FlowTreePage subject="profile" />}
            />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

let mock: FetchMock
afterEach(() => mock?.restore())

describe('Profiles list', () => {
  it('names each profile with its tier, the libraries that use it, its result and its last edit', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/profiles$/,
        body: {
          total: 2,
          items: [
            profileDoc({ used_by: 2, used_by_names: ['Movies', 'TV Shows'] }),
            profileDoc({
              id: 'p2',
              name: 'Archive',
              tier: 'advanced',
              result_policy: 'keep_both',
              used_by: 0,
            }),
          ],
        },
      },
    ])
    renderWith('/encode')
    await screen.findAllByTestId('profile-name-p1')
    expect(screen.getAllByTestId('profile-name-p1')[0]).toHaveTextContent(
      'Films',
    )
    expect(screen.getAllByTestId('profile-tier-p1')[0]).toHaveTextContent(
      'Basic',
    )
    expect(screen.getAllByTestId('profile-tier-p2')[0]).toHaveTextContent(
      'Advanced',
    )
    expect(screen.getAllByTestId('profile-used-p1')[0]).toHaveTextContent(
      '2 libraries',
    )
    expect(screen.getAllByTestId('profile-used-p1')[0]).toHaveTextContent(
      'Movies, TV Shows',
    )
    expect(screen.getAllByTestId('profile-used-p2')[0]).toHaveTextContent(
      'No libraries',
    )
    expect(screen.getAllByText('Replace if smaller').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Keep both').length).toBeGreaterThan(0)
  })

  it('creates a profile at the default tier and opens it', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/profiles$/,
        body: { total: 1, items: [profileDoc()] },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: { graph: PREVIEW_GRAPH, steps: [] },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/profiles$/,
        status: 201,
        bodyFn: ({ body }) =>
          profileDoc({ ...(body as object), id: 'p-new' } as Partial<Profile>),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/profiles\/p-new$/,
        body: profileDoc({ id: 'p-new', name: 'New profile' }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: { items: PALETTE, total: PALETTE.length },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/plugins$/,
        body: { items: [], total: 0 },
      },
    ])
    renderWith('/encode', 'advanced')
    await screen.findAllByTestId('profile-name-p1')
    fireEvent.click(screen.getByTestId('profile-new'))
    await screen.findByTestId('profile-page')
    const created = mock.calls('POST', /\/api\/v1\/profiles$/)
    expect(created).toHaveLength(1)
    const body = created[0].body as {
      name: string
      tier: string
      result_policy: string
    }
    expect(body.name).toBe('New profile')
    expect(body.tier).toBe('advanced')
    expect(body.result_policy).toBe('replace_if_smaller')
  })

  it('duplicates a profile in one click and opens the copy', async () => {
    mock = installFetchMock([
      {
        method: 'GET',
        match: /\/api\/v1\/profiles$/,
        body: { total: 1, items: [profileDoc()] },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/profiles\/p1\/duplicate$/,
        status: 201,
        body: profileDoc({ id: 'p1-copy', name: 'Films (copy)' }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/profiles\/p1-copy$/,
        body: profileDoc({ id: 'p1-copy', name: 'Films (copy)' }),
      },
      {
        method: 'GET',
        match: /\/api\/v1\/flows\/step-types$/,
        body: { items: PALETTE, total: PALETTE.length },
      },
      {
        method: 'GET',
        match: /\/api\/v1\/plugins$/,
        body: { items: [], total: 0 },
      },
      {
        method: 'POST',
        match: /\/api\/v1\/flows\/preview$/,
        body: { graph: PREVIEW_GRAPH, steps: [] },
      },
    ])
    renderWith('/encode')
    fireEvent.click((await screen.findAllByTestId('profile-duplicate-p1'))[0])
    await screen.findByTestId('profile-page')
    expect(mock.calls('POST', /\/profiles\/p1\/duplicate$/)).toHaveLength(1)
    expect(screen.getByTestId('profile-name-input')).toHaveValue('Films (copy)')
  })
})

describe('Profile editor: saves to the profile, per editor', () => {
  it('the Basic form saves the sliders as the profile graph at tier basic', async () => {
    const profile = profileDoc()
    mock = installFetchMock(editorRoutes(profile))
    renderWith('/encode/profiles/p1')
    fireEvent.click(await screen.findByTestId('profile-save'))
    await waitFor(() =>
      expect(mock.calls('PUT', /\/profiles\/p1$/)).toHaveLength(1),
    )
    const body = mock.calls('PUT', /\/profiles\/p1$/)[0].body as {
      name: string
      tier: string
      result_policy: string
      graph: FlowGraph
    }
    expect(body.tier).toBe('basic')
    expect(body.graph).toEqual(PREVIEW_GRAPH)
    expect(body.name).toBe('Films')
    expect(body.result_policy).toBe('replace_if_smaller')
    expect(mock.calls('POST', /\/flows\/preview$/).length).toBeGreaterThan(0)
  })

  it('the tree saves its graph to the profile at tier moderate', async () => {
    const profile = profileDoc({ tier: 'moderate', graph: graphOf(['probe']) })
    mock = installFetchMock(editorRoutes(profile))
    renderWith('/encode/profiles/p1/tree')
    await waitFor(() => screen.getByTestId('tree-rows'))
    const select = screen.getByTestId('tree-add-type')
    fireEvent.change(select, { target: { value: 'skip_if_smaller' } })
    fireEvent.click(screen.getByTestId('tree-add-button'))
    fireEvent.click(screen.getByTestId('tree-save'))
    await waitFor(() =>
      expect(screen.getByText('Profile saved.')).toBeInTheDocument(),
    )
    const puts = mock.calls('PUT', /\/profiles\/p1$/)
    expect(puts).toHaveLength(1)
    const body = puts[0].body as {
      tier: string
      graph: FlowGraph
      name: string
    }
    expect(body.tier).toBe('moderate')
    expect(body.name).toBe('Films')
    expect(body.graph.nodes.map((n) => n.id)).toEqual([
      'probe',
      'skip_if_smaller',
    ])
  })
})

describe('Profile editor: shared profile confirmation and notices', () => {
  it('asks first when libraries use the profile, names them, and saves only on confirm', async () => {
    const profile = profileDoc({
      used_by: 2,
      used_by_names: ['Movies', 'TV Shows'],
    })
    mock = installFetchMock(editorRoutes(profile))
    renderWith('/encode/profiles/p1')
    fireEvent.click(await screen.findByTestId('profile-save'))

    const dialog = await screen.findByTestId('profile-save-dialog')
    const names = within(screen.getByTestId('profile-save-libraries'))
    expect(names.getByText('Movies')).toBeInTheDocument()
    expect(names.getByText('TV Shows')).toBeInTheDocument()
    expect(mock.calls('PUT', /\/profiles\/p1$/)).toHaveLength(0)

    fireEvent.click(within(dialog).getByTestId('profile-save-confirm'))
    await waitFor(() =>
      expect(mock.calls('PUT', /\/profiles\/p1$/)).toHaveLength(1),
    )
  })

  it('names the steps the profile tier cannot express and the tier that can edit them', async () => {
    const profile = profileDoc({
      tier: 'basic',
      unrepresentable: [
        {
          path: 'nodes[1].type',
          message:
            'step "scale_video" needs a scale step: the moderate tier can edit it',
        },
      ],
    })
    mock = installFetchMock(editorRoutes(profile))
    renderWith('/encode/profiles/p1')
    const banner = await screen.findByTestId('tier-banner')
    expect(banner).toHaveTextContent('This profile has a step')
    const item = within(banner).getByTestId('tier-step-item')
    expect(item).toHaveTextContent('scale_video')
    expect(item).toHaveTextContent('can be edited in the Moderate tier')
    expect(screen.getByTestId('tier-banner-advanced')).toHaveAttribute(
      'href',
      '/encode/profiles/p1/flow',
    )
  })

  it('locks the Basic form when the basic tier cannot show a step, and says which tier can', async () => {
    const lockedSteps = [
      {
        path: 'nodes[1].type',
        message:
          'step "raw_1" uses raw arguments: the advanced tier can edit it',
      },
    ]
    mock = installFetchMock(
      editorRoutes(profileDoc({ tier: 'advanced' }), (tier) =>
        tier === 'basic' ? lockedSteps : [],
      ),
    )
    renderWith('/encode/profiles/p1')
    const reason = await screen.findByTestId('profile-basic-locked')
    expect(reason).toHaveTextContent('raw_1')
    expect(reason).toHaveTextContent('Advanced')
    expect(screen.getByTestId('profile-save')).toBeDisabled()
  })

  it('shows the tier that a profile is in use as', async () => {
    mock = installFetchMock(editorRoutes(profileDoc({ tier: 'moderate' })))
    renderWith('/encode/profiles/p1')
    await screen.findByTestId('profile-page')
    expect(screen.getByTestId('tier-moderate-in-use')).toBeInTheDocument()
    expect(screen.queryByTestId('tier-basic-in-use')).toBeNull()
  })
})
