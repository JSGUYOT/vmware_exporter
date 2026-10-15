# vmware-exporter

Exporter Prometheus en Go pour **vCenter 8.x** et **vCenter 9.x (VCF 9 / VVF 9)**.
Il expose l'état complet de l'infrastructure vSphere — ressources, santé, alarmes —
pour les graphes (Grafana) **et** l'alerting (Prometheus / Alertmanager).

- Go 1.25, **govmomi v0.56.0** (API vim25 9.1), client_golang 1.23
- Binaire statique unique, aucune dépendance système, construction 100 % hors ligne (`vendor/`)
- Multi-vCenter (8 et 9 côte à côte), collecte en arrière-plan avec cache : un scrape Prometheus ne déclenche aucun appel au vCenter
- Fourni avec 76 règles d'alerting testées et un tableau de bord Grafana

## Pourquoi un nouvel exporter

| | pryorda/vmware_exporter | prezhdarov/vmware-exporter | **vmware-exporter** |
|---|---|---|---|
| Langage | Python (pyVmomi) | Go | Go (govmomi 0.56) |
| Maintenu / VCF 9 | non / non | partiel | oui, négociation de version 8/9 |
| Périmètre | VM, ESX | ressources (perf) | VM, ESX, clusters, datastores, vDS, resource pools, vSAN, appliance vCenter |
| Alarmes vCenter | non | non | **oui** (toutes entités, 1 seule requête) |
| Santé matérielle (IPMI), multipath, uplinks, certificats | non | non | **oui** |
| Règles d'alerting fournies | non | non | **76 règles + tests promtool** |

## Compatibilité vCenter 8 / vCenter 9 : ce qui est géré

Les différences entre les deux lignes sont traitées explicitement :

1. **Négociation de la version d'API SOAP.** govmomi parle par défaut la dernière API connue (9.1).
   L'exporter lit `/sdk/vimServiceVersions.xml` sur chaque vCenter et utilise *sa* version
   (8.0.x.0 ou 9.x.0.0). Le endpoint vSAN (`/sdk/vsanServiceVersions.xml`) est négocié séparément.
   La version utilisée est visible dans `vmware_vcenter_info{api_version_used=...}`.
2. **Repli automatique sur propriété inconnue.** Si une propriété demandée n'existe pas dans la
   version d'API du vCenter (fault `InvalidProperty`), elle est retirée et la requête rejouée :
   la collecte se dégrade au lieu d'échouer. Compteur : `vmware_exporter_unsupported_properties`.
3. **Aucune dépendance aux fonctions retirées ou dépréciées en 9.x** : vCLS (déprécié en 9.0,
   désactivé en 9.1), SIOC et Storage DRS load balancer (retirés en 9.0), Host Profiles (dépréciés),
   baselines vLCM (retirées), Enhanced Linked Mode (déprécié). Les VMs système (vCLS, Supervisor,
   agents EAM) sont identifiées par `managed_by` et peuvent être exclues (`exclude_system_vms`).
4. **API REST moderne uniquement** (`/api/...`, présente depuis 7.0U2) — jamais l'ancien préfixe
   `/rest/...`, voué à disparaître. Session REST dédiée, réutilisée et renouvelée sur 401.
5. **TLS 1.3** (défaut des vCenter 9) supporté ; `tls_min_version: "1.3"` possible.
6. **Multi-vCenter natif** : avec l'abandon de l'ELM en VCF 9, chaque vCenter de la flotte est
   une cible indépendante (`targets:`), étiquetée par `vcenter` et vos `extra_labels`.
7. **Aucun parsing de noms de produit** (ESXi → ESX en 9) : seules les versions numériques sont lues.

Les tests automatisés rejouent une collecte complète contre le simulateur govmomi annonçant
successivement **8.0.3.0, 9.0.0.0 et 9.1.0.0** (voir « Tests »).

## Ce qui est supervisé

| Domaine | Exemples de métriques | Alertes typiques |
|---|---|---|
| Exporter | `vmware_up`, durée, collecteurs en échec | vCenter injoignable, données figées |
| vCenter | version, certificat TLS, sessions, inventaire | certificat < 30 j |
| **Alarmes vCenter** | `vmware_alarm_triggered{alarm,entity,entity_type,status,acknowledged}` | toute alarme rouge/jaune non acquittée |
| Appliance (VAMI) | santé système/mem/stockage/BDD, services, sauvegardes | service arrêté, sauvegarde en échec ou > 26 h |
| Clusters | capacité/demande CPU-mém, HA, DRS, score DRS, bascule HA | HA désactivé, capacité de bascule perdue, pression ressources |
| ESX | connexion, maintenance, quarantaine, état agent HA, capteurs IPMI, composants matériels, chemins de stockage, uplinks, SSH/NTP, certificat, reboot requis | ESX non répondant, capteur rouge, LUN sans chemin, uplink down |
| VM | power, heartbeat, tools, question en attente, consolidation, snapshots (nombre, âge), disques invités, protection HA, ballooning/swap | heartbeat perdu, snapshot > 3 j, disque invité < 5 %, CPU ready > 10 % |
| Datastores | capacité, libre, provisionné, accessibilité par ESX, maintenance | > 85/95 %, plein dans 7 j, inaccessible |
| Réseau | vDS (synchronisation par ESX), portgroups | ESX outOfSync |
| vSAN | santé globale, par groupe, par test | santé rouge, test en échec |
| Performance temps réel | CPU ready/co-stop/latence, latences disque/datastore, réseau, pertes de paquets, énergie | latence datastore > 30 ms, pertes réseau |

Référence complète : [docs/METRICS.md](docs/METRICS.md).
Convention des statuts : `0 = gray/inconnu, 1 = green, 2 = yellow, 3 = red`.

Les séries VM sont identifiées par `vcenter`, `datacenter`, `vm` et `vm_moid` uniquement : `host` et
`cluster` changent à chaque vMotion et casseraient la continuité des graphes et les délais des
alertes. Ils sont portés par `vmware_vm_info`, à joindre au besoin :
`vmware_vm_cpu_usage_mhz * on(vcenter, vm_moid) group_left(cluster, host) vmware_vm_info`.

## Architecture

```
            ┌──────────── vmware-exporter ────────────┐
vCenter 8 ◄─┤ boucle de collecte (interval)           │
vCenter 9 ◄─┤   1. connexion / reprise de session      │      GET /metrics
   ...      │   2. ContainerView + PropertyCollector   ├──►  (cache, instantané)  ◄── Prometheus
            │   3. collecteurs en parallèle            │
            │   4. compteurs perf (QueryPerf, batchs)  │
            └──────────────────────────────────────────┘
```

Une collecte = un ContainerView sur l'inventaire puis une requête PropertyCollector par type
d'objet (propriétés ciblées, pagination gérée), soit quelques dizaines d'appels quel que soit
le nombre de VMs. Les compteurs temps réel sont demandés par lots (`batch_size`) avec un
parallélisme borné (`max_concurrency`). Les alarmes sont lues en **une seule requête**
(`triggeredAlarmState` du dossier racine, propagé depuis toutes les entités).

## Installation (environnement hors ligne)

Le dépôt contient `vendor/` : aucune connexion Internet n'est nécessaire pour construire.

```bash
# Sur une machine disposant de Go >= 1.25
make build                 # -> dist/vmware-exporter (Linux amd64, statique)
make build-windows         # -> dist/vmware-exporter.exe
```

Un binaire Linux amd64 prêt à l'emploi est aussi fourni dans `dist/`.

```bash
# Déploiement
useradd --system --no-create-home --shell /usr/sbin/nologin vmware-exporter
install -m 0755 dist/vmware-exporter /usr/local/bin/
install -d -m 0750 -o root -g vmware-exporter /etc/vmware-exporter
install -m 0640 -o root -g vmware-exporter config.example.yml /etc/vmware-exporter/config.yml
# Mot de passe du compte de service (jamais dans le YAML)
printf '%s' 'LeMotDePasse' > /etc/vmware-exporter/vcenter.pass
chown root:vmware-exporter /etc/vmware-exporter/vcenter.pass && chmod 0640 /etc/vmware-exporter/vcenter.pass
echo 'VMWARE_EXPORTER_USER=svc-prometheus@vsphere.local' > /etc/vmware-exporter/vmware-exporter.env
# CA du vCenter (VMCA : https://<vcenter>/certs/download.zip) ou de votre PKI
cp ca.pem /etc/vmware-exporter/ca.pem

vmware-exporter -config.file /etc/vmware-exporter/config.yml -config.check
install -m 0644 deploy/systemd/vmware-exporter.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now vmware-exporter
curl -s localhost:9272/metrics | grep '^vmware_up'
```

Image conteneur (optionnelle) : `docker build -t vmware-exporter:1.0.0 .` (construction depuis `vendor/`).

## Compte de service et droits

Créer un compte dédié et lui attribuer le rôle **Read-only** propagé depuis la racine de
l'inventaire (vCenter > Permissions). Cela couvre inventaire, performance, alarmes et santé vSAN.

- **VCF 9 avec fédération d'identité** (VCF Identity Broker, ADFS, Entra ID, Okta…) : utiliser un
  compte **local SSO** (`…@vsphere.local`), l'authentification SOAP par mot de passe n'acceptant
  pas les comptes fédérés.
- `vmware_vcenter_sessions` n'apparaît que si le compte peut lister les sessions
  (privilège *Sessions.TerminateSession*) — facultatif.
- Collecteur `appliance` (API `/api/appliance/...`) : selon la version, certains endpoints
  peuvent exiger un rôle supplémentaire côté appliance. Un endpoint refusé ou absent donne
  `vmware_appliance_endpoint_up{endpoint=...} 0` sans bloquer le reste de la collecte.

## Configuration

Voir [config.example.yml](config.example.yml) (entièrement commenté). Points clés :

- `defaults:` est hérité par chaque cible, tout est surchargeable par cible.
- Secrets : `password_file` (recommandé) ou `password_env` ; `${VAR}` est interpolé partout.
- `interval` (défaut 60 s, minimum 20 s) et `timeout` (défaut `interval - 5s`).
- `collectors:` active/désactive chaque famille ; `filters:` restreint par regex.
- `perf.host_counters` / `perf.vm_counters` : compteurs vSphere `groupe.compteur.rollup`,
  suffixe `:*` pour toutes les instances (ex. `datastore.totalReadLatency.average:*`).
- `extra_labels:` ajoute des labels statiques (site, env…) à toutes les séries.

Compteurs collectés par défaut :

- **ESX** : cpu.usage/utilization/coreUtilization, mem.usage/active/consumed/vmmemctl/swapinRate/swapoutRate,
  disk.maxTotalLatency/read/write/commandsAborted/busResets, net.received/transmitted/droppedRx/droppedTx/errorsRx/errorsTx,
  power.power, datastore.totalReadLatency/totalWriteLatency (par datastore)
- **VM** : cpu.usage/ready/costop/latency/readiness, mem.usage/active/vmmemctl/swapped/swapinRate,
  disk.maxTotalLatency/read/write, net.received/transmitted/droppedRx/droppedTx

Nom Prometheus : `vmware_<host|vm>_perf_<groupe>_<compteur>_<rollup>_<unité>` (pourcentages en 0–100,
octets et secondes en unités de base), label `perf_instance` (`""` = agrégat).
Exemple : CPU ready en % par vCPU = `vmware_vm_perf_cpu_ready_summation_seconds{perf_instance=""} / 20 / ignoring(perf_instance) vmware_vm_vcpus`.

## Endpoints HTTP

| Chemin | Rôle |
|---|---|
| `/metrics` | métriques (cache de la dernière collecte de chaque vCenter) |
| `/healthz` | vivacité du processus |
| `/` | page d'accueil (liste des cibles) |

## Alerting

[deploy/prometheus/vmware-alerts.yml](deploy/prometheus/vmware-alerts.yml) : 76 règles en 8 groupes
(exporter, vCenter + appliance, cluster, ESX + performance, VM, datastore, réseau, vSAN).

- **Relais des alarmes vCenter** : `VMwareVCenterAlarmCritical` / `VMwareVCenterAlarmWarning`
  transforment chaque alarme native active et non acquittée en alerte Prometheus (label `source=vcenter`).
  Tout ce que le vCenter sait détecter arrive ainsi dans Alertmanager sans rien redéfinir ;
  acquitter l'alarme dans le vSphere Client fait disparaître l'alerte.
- **Règles propres à Prometheus** pour ce que les alarmes vCenter couvrent mal : tendances
  (datastore plein dans 7 j), redondance (multipath, uplinks), CPU ready, snapshots anciens,
  sauvegarde du vCenter, collecte en échec…
- Sévérités : `critical`, `warning`, `info`. Les seuils sont des valeurs de départ à ajuster.

```bash
promtool check rules deploy/prometheus/vmware-alerts.yml
cd deploy/prometheus && promtool test rules vmware-alerts.test.yml
```

Extrait de configuration Prometheus : [deploy/prometheus/prometheus-scrape.yml](deploy/prometheus/prometheus-scrape.yml).

## Grafana

Importer [deploy/grafana/vmware-overview.json](deploy/grafana/vmware-overview.json) (Grafana ≥ 10) :
vue d'ensemble, alarmes vCenter actives, clusters, ESX, datastores, VMs (top CPU ready, latences,
snapshots, disques invités), vSAN, santé de l'exporter. Variables : source, vCenter, datacenter, cluster.
Le JSON est généré par `deploy/grafana/build_dashboard.py` pour faciliter les modifications.

## Tests

```bash
make test        # tests Go : matrice 8.0.3 / 9.0 / 9.1 sur simulateur, reprise de session,
                 # vCenter injoignable, API REST appliance simulée, lint des noms de métriques
make test-rules  # tests unitaires des règles d'alerting (promtool)
make sim         # lance un vCenter simulé sur https://127.0.0.1:8989/sdk (user / pass)
```

Le simulateur (`hack/vcsim`) permet d'essayer l'exporter, Prometheus et Grafana sans
infrastructure : `make sim VERSION_SIM=8.0.3 API_SIM=8.0.3.0` puis pointer une cible sur
`127.0.0.1:8989` (`username: user`, mot de passe `pass`, `insecure_skip_verify: true`).

## Limites connues et validation

- Les tests automatisés utilisent le simulateur govmomi, pas un vCenter réel : valider d'abord sur
  une préproduction 8.x et 9.x. Surveiller au démarrage les logs `propriété non supportée` et
  `collecteur en échec`, ainsi que `vmware_exporter_unsupported_properties` (propriété absente de la
  version d'API) et `vmware_exporter_missing_properties` (propriété refusée sur certains objets, ex. droits).
- Compteurs perf : moyenne des échantillons 20 s depuis la dernière collecte, sauf rollups
  `latest` (dernier échantillon) et `maximum`/`minimum` (extrême, les pics ne sont pas lissés).
- Le format exact de `/api/appliance/recovery/backup/job/details` peut varier selon la version ;
  à défaut, les métriques de sauvegarde sont simplement absentes.
- La santé vSAN est lue depuis le cache du health service (pas de relance des tests) : sa
  fraîcheur dépend de la périodicité configurée côté vSAN (`vmware_vsan_health_timestamp_seconds`).
- Hors périmètre (API distinctes du vCenter) : SDDC Manager, NSX, VCF Operations, VCF Automation.

## Structure du dépôt

```
cmd/vmware-exporter/     point d'entrée (HTTP, registry, signaux)
internal/config/         chargement / validation YAML, secrets, valeurs par défaut
internal/vsphere/        connexion SOAP (négociation de version, reprise de session), client REST /api
internal/collector/      collecteurs (vcenter, alarms, cluster, host, vm, datastore, network,
                         resourcepool, perf, vsan, appliance) + cache + tests
deploy/prometheus/       règles d'alerting, tests promtool, extrait de scrape
deploy/grafana/          tableau de bord (JSON + générateur)
deploy/systemd/          unité systemd durcie
hack/vcsim/              vCenter simulé (8.x / 9.x) pour les essais
docs/METRICS.md          référence des métriques
vendor/                  dépendances (construction hors ligne)
```
