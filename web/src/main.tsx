import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

type SkillStatus = 'enabled' | 'disabled' | 'invalid';
type Source = 'user' | 'system' | 'plugin';
type Filter = 'all' | SkillStatus;
type Tab = 'skills' | 'plugins';

type ConfigEntry = {
  name?: string;
  path?: string;
  enabled: boolean;
};

type Skill = {
  name: string;
  title: string;
  description?: string;
  source: Source;
  status: SkillStatus;
  path: string;
  skillFilePath: string;
  hasSkillFile: boolean;
  configEntry: ConfigEntry | null;
  legacyDisabled?: boolean;
  pluginId?: string;
};

type PluginSkill = {
  name: string;
  title: string;
  description?: string;
  path: string;
  enabledByPlugin: boolean;
  individuallyDisabled: boolean;
};

type Plugin = {
  id: string;
  name: string;
  marketplace: string;
  enabled: boolean;
  configPath: string;
  cachePaths: string[];
  skills: PluginSkill[];
};

type SkillGroup = {
  id: string;
  title: string;
  source: Source;
  skills: Skill[];
  enabled: number;
  disabled: number;
  invalid: number;
};

const filterLabels: Record<Filter, string> = {
  all: '全部',
  enabled: '已启用',
  disabled: '已禁用',
  invalid: '异常',
};

const sourceLabels: Record<Source, string> = {
  user: '用户',
  system: '系统',
  plugin: '插件',
};

function App() {
  const [tab, setTab] = useState<Tab>('skills');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [expandedPluginIds, setExpandedPluginIds] = useState<Set<string>>(new Set());
  const [expandedSkillGroupIds, setExpandedSkillGroupIds] = useState<Set<string>>(new Set(['user', 'system']));
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const [loading, setLoading] = useState(true);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [error, setError] = useState('');

  async function loadAll() {
    setLoading(true);
    setError('');
    try {
      const [skillsResponse, pluginsResponse] = await Promise.all([
        fetch('/api/skills'),
        fetch('/api/plugins'),
      ]);
      if (!skillsResponse.ok) throw new Error(await readError(skillsResponse));
      if (!pluginsResponse.ok) throw new Error(await readError(pluginsResponse));
      setSkills(await skillsResponse.json());
      setPlugins(await pluginsResponse.json());
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取数据失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadAll();
  }, []);

  const pluginStats = useMemo(() => {
    return plugins.reduce(
      (acc, plugin) => {
        if (plugin.enabled) acc.enabled += 1;
        else acc.disabled += 1;
        acc.skills += plugin.skills.length;
        return acc;
      },
      { enabled: 0, disabled: 0, skills: 0 },
    );
  }, [plugins]);

  const stats = useMemo(() => {
    return skills.reduce(
      (acc, skill) => {
        acc[skill.status] += 1;
        if (skill.legacyDisabled) acc.legacy += 1;
        return acc;
      },
      { enabled: 0, disabled: 0, invalid: 0, legacy: 0 },
    );
  }, [skills]);

  const visibleSkills = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return skills.filter((skill) => {
      const matchesFilter = filter === 'all' || skill.status === filter;
      const searchable = [
        skill.name,
        skill.title,
        skill.description ?? '',
        skill.path,
        skill.skillFilePath,
        skill.source,
        skill.pluginId ?? '',
      ]
        .join(' ')
        .toLowerCase();
      return matchesFilter && (!keyword || searchable.includes(keyword));
    });
  }, [skills, query, filter]);

  async function runSkillAction(skill: Skill, action: 'enable' | 'disable' | 'delete' | 'migrate-legacy-disabled') {
    if (action === 'delete') {
      const confirmed = window.confirm(
        `确认软删除这个用户技能？\n\n${skill.path}\n\n目录会移动到 /Users/spc/.codex/skills.trash/，不会直接永久删除。`,
      );
      if (!confirmed) return;
    }
    if (action === 'migrate-legacy-disabled') {
      const confirmed = window.confirm(
        `确认迁移历史禁用技能？\n\n${skill.path}\n\n将移回 /Users/spc/.codex/skills/${skill.name}，并在 config.toml 中写入 enabled = false。`,
      );
      if (!confirmed) return;
    }

    const key = `skill:${skill.name}:${action}`;
    setBusyKey(key);
    setError('');
    try {
      const response = await fetch(
        action === 'delete'
          ? `/api/skills/${encodeURIComponent(skill.name)}`
          : `/api/skills/${encodeURIComponent(skill.name)}/${action}`,
        { method: action === 'delete' ? 'DELETE' : 'POST' },
      );
      if (!response.ok) throw new Error(await readError(response));
      await loadAll();
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败');
    } finally {
      setBusyKey(null);
    }
  }

  async function runPluginAction(plugin: Plugin, action: 'enable' | 'disable') {
    if (action === 'disable') {
      const confirmed = window.confirm(
        `确认禁用插件？\n\n${plugin.id}\n\n禁用插件可能会隐藏该插件带来的 skills、tools、connectors、apps、hooks。`,
      );
      if (!confirmed) return;
    }
    const key = `plugin:${plugin.id}:${action}`;
    setBusyKey(key);
    setError('');
    try {
      const response = await fetch(`/api/plugins/${encodeURIComponent(plugin.id)}/${action}`, {
        method: 'POST',
      });
      if (!response.ok) throw new Error(await readError(response));
      await loadAll();
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败');
    } finally {
      setBusyKey(null);
    }
  }

  function togglePluginDetails(id: string) {
    setExpandedPluginIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleSkillGroup(id: string) {
    setExpandedSkillGroupIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <main className="app-frame">
      <aside className="command-rail" aria-label="Codex 控制区">
        <div className="brand-lockup">
          <span className="brand-mark">SM</span>
          <div>
            <h1>Skill Manager</h1>
            <p>Codex 本地开关台</p>
          </div>
        </div>

        <nav className="rail-tabs" aria-label="管理对象">
          <button className={tab === 'skills' ? 'active' : ''} onClick={() => setTab('skills')}>
            <span>技能</span>
            <strong>{skills.length}</strong>
          </button>
          <button className={tab === 'plugins' ? 'active' : ''} onClick={() => setTab('plugins')}>
            <span>插件</span>
            <strong>{plugins.length}</strong>
          </button>
        </nav>

        <section className="rail-panel" aria-label="当前状态">
          <div>
            <span>配置来源</span>
            <code>/Users/spc/.codex/config.toml</code>
          </div>
          <div>
            <span>技能目录</span>
            <code>/Users/spc/.codex/skills</code>
          </div>
        </section>

        <section className="rail-metrics" aria-label="汇总统计">
          <MiniMetric label="启用技能" value={stats.enabled} />
          <MiniMetric label="启用插件" value={pluginStats.enabled} />
          <MiniMetric label="异常项" value={stats.invalid} tone={stats.invalid > 0 ? 'warning' : 'muted'} />
          <MiniMetric label="历史目录" value={stats.legacy} tone={stats.legacy > 0 ? 'warning' : 'muted'} />
        </section>

        <button className="refresh-button" onClick={loadAll} disabled={loading}>
          {loading ? '同步中' : '刷新数据'}
        </button>
      </aside>

      <section className="workspace">
        <header className="workspace-header">
          <div>
            <p className="section-kicker">LOCAL CODEX CONTROL</p>
            <h2>{tab === 'skills' ? '技能运行面板' : '插件运行面板'}</h2>
            <p>
              {tab === 'skills'
                ? '按来源折叠管理技能，启用和禁用只写入配置，不移动目录。'
                : '查看插件配置、缓存路径和随插件加载的技能。'}
            </p>
          </div>
          <div className="header-readout" aria-label="当前读数">
            <span>{loading ? '读取中' : '已读取'}</span>
            <strong>{tab === 'skills' ? `${visibleSkills.length}/${skills.length}` : String(plugins.length)}</strong>
          </div>
        </header>

        {error && <div className="error-banner">{error}</div>}

        {tab === 'skills' ? (
          <SkillsView
            skills={visibleSkills}
            stats={stats}
            query={query}
            filter={filter}
            loading={loading}
            busyKey={busyKey}
            expandedSkillGroupIds={expandedSkillGroupIds}
            setQuery={setQuery}
            setFilter={setFilter}
            toggleSkillGroup={toggleSkillGroup}
            runSkillAction={runSkillAction}
          />
        ) : (
          <PluginsView
            plugins={plugins}
            loading={loading}
            busyKey={busyKey}
            expandedPluginIds={expandedPluginIds}
            togglePluginDetails={togglePluginDetails}
            runPluginAction={runPluginAction}
          />
        )}
      </section>
    </main>
  );
}

function SkillsView({
  skills,
  stats,
  query,
  filter,
  loading,
  busyKey,
  expandedSkillGroupIds,
  setQuery,
  setFilter,
  toggleSkillGroup,
  runSkillAction,
}: {
  skills: Skill[];
  stats: { enabled: number; disabled: number; invalid: number; legacy: number };
  query: string;
  filter: Filter;
  loading: boolean;
  busyKey: string | null;
  expandedSkillGroupIds: Set<string>;
  setQuery: (value: string) => void;
  setFilter: (value: Filter) => void;
  toggleSkillGroup: (id: string) => void;
  runSkillAction: (skill: Skill, action: 'enable' | 'disable' | 'delete' | 'migrate-legacy-disabled') => void;
}) {
  const groups = useMemo(() => groupSkills(skills), [skills]);
  const forceExpanded = query.trim() !== '' || filter !== 'all';

  return (
    <>
      <section className="stats-grid" aria-label="技能统计">
        <Stat label="已启用" value={stats.enabled} tone="enabled" />
        <Stat label="已禁用" value={stats.disabled} tone="disabled" />
        <Stat label="异常" value={stats.invalid} tone="invalid" />
        <Stat label="历史禁用目录" value={stats.legacy} tone="legacy" />
      </section>

      <section className="tool-strip" aria-label="筛选工具">
        <label className="search-box">
          <span>搜索</span>
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="名称、描述、来源或路径"
            aria-label="搜索技能"
          />
        </label>
        <div>
          <span className="control-label">状态</span>
          <div className="segments" aria-label="状态筛选">
            {(Object.keys(filterLabels) as Filter[]).map((item) => (
              <button key={item} className={filter === item ? 'active' : ''} onClick={() => setFilter(item)}>
                {filterLabels[item]}
              </button>
            ))}
          </div>
        </div>
      </section>

      <section className="list-header panel-header">
        <strong>技能清单</strong>
        <span>{loading ? '读取中' : `共 ${groups.length} 组 / ${skills.length} 个技能`}</span>
      </section>

      <section className="records" aria-label="技能列表">
        {loading ? (
          <LoadingRows />
        ) : (
          <>
            {groups.map((group) => {
              const expanded = forceExpanded || expandedSkillGroupIds.has(group.id);
              return (
                <article className="record-group" key={group.id}>
                  <div className="group-header">
                    <div className="skill-main">
                      <div className="title-line">
                        <h3>{group.title}</h3>
                        <span className={`source ${group.source}`}>{sourceLabels[group.source]}</span>
                        <span className="count-chip">{group.skills.length} 个技能</span>
                      </div>
                      <div className="group-metrics">
                        <span>启用 {group.enabled}</span>
                        <span>禁用 {group.disabled}</span>
                        <span>异常 {group.invalid}</span>
                      </div>
                    </div>
                    <button className="compact-button" onClick={() => toggleSkillGroup(group.id)}>
                      {expanded ? '收起' : '展开'}
                    </button>
                  </div>

                  {expanded && (
                    <div className="record-children">
                      {group.skills.map((skill) => (
                        <SkillRow
                          key={`${skill.source}-${skill.pluginId ?? ''}-${skill.path}`}
                          skill={skill}
                          busyKey={busyKey}
                          runSkillAction={runSkillAction}
                        />
                      ))}
                    </div>
                  )}
                </article>
              );
            })}
            {groups.length === 0 && <div className="empty-state">没有匹配的技能</div>}
          </>
        )}
      </section>
    </>
  );
}

function SkillRow({
  skill,
  busyKey,
  runSkillAction,
}: {
  skill: Skill;
  busyKey: string | null;
  runSkillAction: (skill: Skill, action: 'enable' | 'disable' | 'delete' | 'migrate-legacy-disabled') => void;
}) {
  return (
    <article className="record-row child-row">
      <div className="skill-main">
        <div className="title-line">
          <h4>{skill.title}</h4>
          <span className={`status ${skill.status}`}>{statusLabel(skill.status)}</span>
          {skill.legacyDisabled && <span className="source legacy">历史禁用目录</span>}
        </div>
        <p className="description">{skill.description || '未提供描述'}</p>
        <code>{skill.skillFilePath || skill.path}</code>
        {skill.configEntry && (
          <p className="config-note">
            config: {skill.configEntry.path || skill.configEntry.name} / enabled = {String(skill.configEntry.enabled)}
          </p>
        )}
      </div>
      <div className="actions">
        {skill.legacyDisabled ? (
          <button
            className="primary-action"
            disabled={busyKey === `skill:${skill.name}:migrate-legacy-disabled`}
            onClick={() => runSkillAction(skill, 'migrate-legacy-disabled')}
          >
            迁移
          </button>
        ) : skill.status !== 'invalid' ? (
          skill.status === 'enabled' ? (
            <button className="muted-action" disabled={busyKey === `skill:${skill.name}:disable`} onClick={() => runSkillAction(skill, 'disable')}>
              禁用
            </button>
          ) : (
            <button className="primary-action" disabled={busyKey === `skill:${skill.name}:enable`} onClick={() => runSkillAction(skill, 'enable')}>
              启用
            </button>
          )
        ) : null}
        {skill.source === 'user' && !skill.legacyDisabled && (
          <button
            className="danger-button"
            disabled={busyKey === `skill:${skill.name}:delete`}
            onClick={() => runSkillAction(skill, 'delete')}
          >
            软删除
          </button>
        )}
      </div>
    </article>
  );
}

function PluginsView({
  plugins,
  loading,
  busyKey,
  expandedPluginIds,
  togglePluginDetails,
  runPluginAction,
}: {
  plugins: Plugin[];
  loading: boolean;
  busyKey: string | null;
  expandedPluginIds: Set<string>;
  togglePluginDetails: (id: string) => void;
  runPluginAction: (plugin: Plugin, action: 'enable' | 'disable') => void;
}) {
  const enabledPlugins = plugins.filter((plugin) => plugin.enabled).length;
  const pluginSkillCount = plugins.reduce((sum, plugin) => sum + plugin.skills.length, 0);

  return (
    <>
      <section className="stats-grid" aria-label="插件统计">
        <Stat label="插件总数" value={plugins.length} tone="enabled" />
        <Stat label="已启用" value={enabledPlugins} tone="enabled" />
        <Stat label="已禁用" value={plugins.length - enabledPlugins} tone="disabled" />
        <Stat label="插件技能" value={pluginSkillCount} tone="legacy" />
      </section>

      <section className="records" aria-label="插件列表">
        <section className="list-header panel-header">
          <strong>插件清单</strong>
          <span>{loading ? '读取中' : `共 ${plugins.length} 个`}</span>
        </section>
        {loading ? (
          <LoadingRows />
        ) : (
          <>
            {plugins.map((plugin) => {
              const expanded = expandedPluginIds.has(plugin.id);
              return (
                <article className="record-row plugin-row" key={plugin.id}>
                  <div className="skill-main">
                    <div className="title-line">
                      <h3>{plugin.id}</h3>
                      <span className={`status ${plugin.enabled ? 'enabled' : 'disabled'}`}>
                        {plugin.enabled ? '已启用' : '已禁用'}
                      </span>
                    </div>
                    <p className="description">
                      marketplace: {plugin.marketplace} / cachePaths: {plugin.cachePaths.length} / skills: {plugin.skills.length}
                    </p>
                    <code>{plugin.configPath}</code>
                    {expanded && (
                      <div className="plugin-skills">
                        {plugin.skills.map((skill) => (
                          <div className="plugin-skill" key={skill.path}>
                            <div>
                              <strong>{skill.title}</strong>
                              <p>{skill.description || '未提供描述'}</p>
                              <code>{skill.path}</code>
                            </div>
                            <div className="plugin-flags">
                              <span>enabledByPlugin = {String(skill.enabledByPlugin)}</span>
                              <span>individuallyDisabled = {String(skill.individuallyDisabled)}</span>
                            </div>
                          </div>
                        ))}
                        {plugin.skills.length === 0 && <div className="empty-inline">未扫描到插件技能</div>}
                      </div>
                    )}
                  </div>
                  <div className="actions">
                    <button className="compact-button" onClick={() => togglePluginDetails(plugin.id)}>
                      {expanded ? '收起' : '详情'}
                    </button>
                    {plugin.enabled ? (
                      <button
                        className="muted-action"
                        disabled={busyKey === `plugin:${plugin.id}:disable`}
                        onClick={() => runPluginAction(plugin, 'disable')}
                      >
                        禁用
                      </button>
                    ) : (
                      <button
                        className="primary-action"
                        disabled={busyKey === `plugin:${plugin.id}:enable`}
                        onClick={() => runPluginAction(plugin, 'enable')}
                      >
                        启用
                      </button>
                    )}
                  </div>
                </article>
              );
            })}
            {plugins.length === 0 && <div className="empty-state">config.toml 中没有插件配置</div>}
          </>
        )}
      </section>
    </>
  );
}

function MiniMetric({ label, value, tone = 'normal' }: { label: string; value: number; tone?: string }) {
  return (
    <div className={`mini-metric ${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function LoadingRows() {
  return (
    <div className="loading-stack" aria-label="读取中">
      {[0, 1, 2].map((item) => (
        <div className="skeleton-row" key={item}>
          <span />
          <span />
          <span />
        </div>
      ))}
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className={`stat-card ${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function groupSkills(skills: Skill[]): SkillGroup[] {
  const groups = new Map<string, SkillGroup>();

  for (const skill of skills) {
    const id =
      skill.source === 'plugin'
        ? `plugin:${skill.pluginId || 'unknown'}`
        : skill.source === 'user'
          ? 'user'
          : 'system';
    const title =
      skill.source === 'plugin'
        ? skill.pluginId || '未知插件'
        : skill.source === 'user'
          ? '用户自装技能'
          : '系统技能';

    if (!groups.has(id)) {
      groups.set(id, {
        id,
        title,
        source: skill.source,
        skills: [],
        enabled: 0,
        disabled: 0,
        invalid: 0,
      });
    }

    const group = groups.get(id)!;
    group.skills.push(skill);
    group[skill.status] += 1;
  }

  return Array.from(groups.values())
    .map((group) => ({
      ...group,
      skills: group.skills.sort((a, b) => a.title.localeCompare(b.title)),
    }))
    .sort((a, b) => {
      const order = { plugin: 0, user: 1, system: 2 };
      if (order[a.source] !== order[b.source]) return order[a.source] - order[b.source];
      return a.title.localeCompare(b.title);
    });
}

function statusLabel(status: SkillStatus) {
  if (status === 'enabled') return '已启用';
  if (status === 'disabled') return '已禁用';
  return '异常';
}

async function readError(response: Response) {
  try {
    const body = await response.json();
    return body.error || response.statusText;
  } catch {
    return response.statusText;
  }
}

createRoot(document.getElementById('root')!).render(<App />);
