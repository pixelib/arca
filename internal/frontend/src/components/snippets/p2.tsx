import { CodeBlock } from '@/components/copy-button'
import type { RepositoryDetail } from '@/lib/api'

const proxyExample = 'releases/2024-12'
const hostedExample = 'commons/25.2.5'

export function P2Usage({ repository }: { repository: RepositoryDetail }) {
  const base = `${window.location.origin}/repository/${repository.name}`
  const isProxy = repository.type === 'proxy'
  const isPrivate = repository.visibility === 'private'
  const site = `${base}/${isProxy ? proxyExample : hostedExample}`

  const target = `<location includeMode="slicer" type="InstallableUnit">
  <repository location="${site}"/>
  <unit id="${isProxy ? 'org.eclipse.jgit.feature.group' : 'com.example.commons'}" version="0.0.0"/>
</location>`

  const tycho = `<repositories>
  <repository>
    <id>${repository.name}</id>
    <url>${site}</url>
    <layout>p2</layout>
  </repository>
</repositories>`

  const publish = `<plugin>
  <groupId>org.codehaus.mojo</groupId>
  <artifactId>wagon-maven-plugin</artifactId>
  <executions>
    <execution>
      <phase>package</phase>
      <goals><goal>upload</goal></goals>
      <configuration>
        <serverId>${repository.name}</serverId>
        <fromDir>\${project.build.directory}/repository</fromDir>
        <includes>**</includes>
        <url>${base}</url>
        <toDir>${hostedExample}</toDir>
      </configuration>
    </execution>
  </executions>
</plugin>`

  const settings = `<servers>
  <server>
    <id>${repository.name}</id>
    <username>YOUR_EMAIL</username>
    <password>YOUR_API_TOKEN</password>
  </server>
</servers>`

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">Update site</p>
        <p className="max-w-2xl text-sm text-muted-foreground">
          {isProxy
            ? 'This proxy mirrors the whole upstream host, so append the path of the site you want. In Eclipse, paste the URL into Help, Install New Software.'
            : 'Each published site lives under its own path. Paste the URL into Help, Install New Software, or point a target definition at it.'}
          {isPrivate ? ' This repository is private, so Eclipse prompts for your email and an API token.' : ' Reading needs no credentials.'}
        </p>
      </div>
      <CodeBlock language="Update site" code={site} />

      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">Target platform</p>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Add the site to a PDE target definition to build against it.
        </p>
      </div>
      <CodeBlock language=".target" code={target} />

      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">Tycho</p>
        <p className="max-w-2xl text-sm text-muted-foreground">
          A Tycho build resolves the target platform from a repository marked with the p2 layout.
        </p>
      </div>
      <CodeBlock language="pom.xml" code={tycho} />

      {isProxy ? null : (
        <>
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">Publishing</p>
            <p className="max-w-2xl text-sm text-muted-foreground">
              A Tycho <code className="font-mono text-xs">eclipse-repository</code> build writes a
              complete site to <code className="font-mono text-xs">target/repository</code>. Upload
              that directory as it stands; republishing over an existing path is allowed, because a
              rebuild rewrites the metadata documents under the same names.
            </p>
          </div>
          <CodeBlock language="pom.xml" code={publish} />
          <CodeBlock language="settings.xml" code={settings} />
        </>
      )}
    </div>
  )
}
