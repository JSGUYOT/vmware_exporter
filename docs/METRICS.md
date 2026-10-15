# Référence des métriques

Toutes les séries portent le label `vcenter` (nom de la cible) et les `extra_labels` configurés.
Statuts vSphere : 0 = gray/inconnu, 1 = green, 2 = yellow, 3 = red. (*) = n'apparaît que sur infrastructure réelle (alarme active, vSAN, appliance, snapshots...).

Les séries VM ne portent pas `host`/`cluster` (stables au vMotion) : voir `vmware_vm_info`.

Compteurs de performance temps réel : `vmware_<host|vm>_perf_<groupe>_<compteur>_<rollup>_<unité>` (ex. `vmware_vm_perf_cpu_ready_summation_seconds`), label `perf_instance` (`""` = agrégat). Pourcentages en 0-100, octets et secondes en unités de base.

## Exporter

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_collection_duration_seconds` | gauge |  | Durée totale de la dernière collecte. |
| `vmware_collection_failures_total` | counter |  | Nombre de collectes en échec (vCenter injoignable / auth). |
| `vmware_collection_timestamp_seconds` | gauge |  | Horodatage de la dernière collecte. |
| `vmware_collections_total` | counter |  | Nombre de collectes effectuées. |
| `vmware_collector_duration_seconds` | gauge | collector | Durée du collecteur lors de la dernière collecte. |
| `vmware_collector_success` | gauge | collector | 1 si le collecteur a réussi lors de la dernière collecte. |
| `vmware_exporter_build_info` | gauge | commit, govmomi_api, version | Version de l'exporter (valeur toujours 1). |
| `vmware_exporter_duplicate_series` | gauge |  | Séries ignorées car dupliquées (objets homonymes). |
| `vmware_exporter_invalid_series` | gauge |  | Séries invalides ignorées (ex. conflit de nom de label). |
| `vmware_exporter_missing_properties` | gauge |  | Propriétés indisponibles sur certains objets lors de la dernière collecte (droits, ESX injoignable...). |
| `vmware_exporter_unsupported_properties` | gauge |  | Propriétés ignorées car absentes de la version d'API du vCenter. |
| `vmware_up` | gauge |  | 1 si la dernière collecte du vCenter a réussi (connexion + inventaire). |

## vCenter

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_inventory_objects` | gauge | type | Nombre d'objets d'inventaire par type. |
| `vmware_vcenter_certificate_expiry_timestamp_seconds` | gauge |  | Date d'expiration du certificat TLS du vCenter (epoch). |
| `vmware_vcenter_info` | gauge | api_version_server, api_version_used, build, instance_uuid, product, product_line, version | Informations sur le vCenter (valeur toujours 1). |
| `vmware_vcenter_sessions` | gauge |  | Nombre de sessions vCenter ouvertes (si le compte a le droit de les lister). |
| `vmware_vcenter_version_major` | gauge |  | Version majeure du vCenter (8 = vCenter 8, 9 = VCF/vSphere 9). |

## Alarmes vCenter

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_alarms_active` | gauge | status | Nombre d'alarmes vCenter actives par sévérité. |
| `vmware_alarm_triggered` (*) | gauge | alarm, alarm_moid, entity, entity_type, entity_moid, datacenter, status, acknowledged | Alarme vCenter active sur une entité, valeur = sévérité (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_alarm_triggered_timestamp_seconds` (*) | gauge | idem vmware_alarm_triggered | Horodatage de déclenchement de l'alarme vCenter. |

## Appliance vCenter (REST /api)

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_appliance_endpoint_up` | gauge | endpoint | 1 si l'endpoint REST de l'appliance a répondu (0 = absent sur cette version ou droits insuffisants). |
| `vmware_appliance_backup_jobs` (*) | gauge |  | Nombre de jobs de sauvegarde file-based connus. |
| `vmware_appliance_backup_last_succeeded` (*) | gauge |  | 1 si la dernière sauvegarde du vCenter a réussi. |
| `vmware_appliance_backup_last_success_timestamp_seconds` (*) | gauge |  | Fin de la dernière sauvegarde réussie du vCenter. |
| `vmware_appliance_health` (*) | gauge | component | Santé d'un composant de l'appliance vCenter (0=gray/inconnu, 1=green, 2=yellow, 3=red) (orange compté comme 2). |
| `vmware_appliance_info` (*) | gauge | version, build, product, type | Version de l'appliance vCenter (valeur toujours 1). |
| `vmware_appliance_service_health` (*) | gauge | service, startup_type | Santé du service vCenter (0=inconnu, 1=healthy, 2=warnings, 3=degraded). |
| `vmware_appliance_service_started` (*) | gauge | service, startup_type | 1 si le service vCenter est démarré. |
| `vmware_appliance_uptime_seconds` (*) | gauge |  | Uptime de l'appliance vCenter. |

## Datacenter

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_datacenter_overall_status` | gauge | datacenter | Statut global du datacenter (0=gray/inconnu, 1=green, 2=yellow, 3=red). |

## Cluster

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_cluster_config_issues` | gauge | cluster, datacenter | Nombre de problèmes de configuration signalés sur le cluster. |
| `vmware_cluster_cpu_cores` | gauge | cluster, datacenter | Nombre de coeurs physiques. |
| `vmware_cluster_cpu_demand_mhz` | gauge | cluster, datacenter | Demande CPU des VMs (MHz). |
| `vmware_cluster_cpu_effective_mhz` | gauge | cluster, datacenter | Capacité CPU effective disponible pour les VMs (MHz). |
| `vmware_cluster_cpu_entitled_mhz` | gauge | cluster, datacenter | CPU attribué aux VMs (MHz). |
| `vmware_cluster_cpu_reserved_mhz` | gauge | cluster, datacenter | CPU réservé (MHz). |
| `vmware_cluster_cpu_threads` | gauge | cluster, datacenter | Nombre de threads physiques. |
| `vmware_cluster_cpu_total_mhz` | gauge | cluster, datacenter | Capacité CPU totale du cluster (MHz). |
| `vmware_cluster_datastores` | gauge | cluster, datacenter | Nombre de datastores montés sur le cluster. |
| `vmware_cluster_drs_enabled` | gauge | cluster, datacenter | 1 si DRS est activé. |
| `vmware_cluster_drs_fully_automated` | gauge | cluster, datacenter | 1 si DRS est en mode entièrement automatisé. |
| `vmware_cluster_drs_recommendations` | gauge | cluster, datacenter | Nombre de recommandations DRS en attente. |
| `vmware_cluster_ha_admission_control_enabled` | gauge | cluster, datacenter | 1 si le contrôle d'admission HA est activé. |
| `vmware_cluster_ha_admission_control_policy` | gauge | cluster, datacenter, policy | Politique de contrôle d'admission HA (label policy, valeur 1). |
| `vmware_cluster_ha_enabled` | gauge | cluster, datacenter | 1 si vSphere HA est activé. |
| `vmware_cluster_ha_host_monitoring_enabled` | gauge | cluster, datacenter | 1 si la surveillance des ESX par HA est activée. |
| `vmware_cluster_hosts` | gauge | cluster, datacenter | Nombre d'ESX dans le cluster. |
| `vmware_cluster_hosts_effective` | gauge | cluster, datacenter | Nombre d'ESX effectifs (connectés, hors maintenance). |
| `vmware_cluster_memory_demand_bytes` | gauge | cluster, datacenter | Demande mémoire des VMs. |
| `vmware_cluster_memory_effective_bytes` | gauge | cluster, datacenter | Mémoire effective disponible pour les VMs. |
| `vmware_cluster_memory_entitled_bytes` | gauge | cluster, datacenter | Mémoire attribuée aux VMs. |
| `vmware_cluster_memory_reserved_bytes` | gauge | cluster, datacenter | Mémoire réservée. |
| `vmware_cluster_memory_total_bytes` | gauge | cluster, datacenter | Mémoire totale du cluster. |
| `vmware_cluster_overall_status` | gauge | cluster, datacenter | Statut global du cluster (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_cluster_vmotions_total` | counter | cluster, datacenter | Nombre de vMotions effectués dans le cluster. |
| `vmware_cluster_vms` | gauge | cluster, datacenter | Nombre total de VMs. |
| `vmware_cluster_vms_powered_on` | gauge | cluster, datacenter | Nombre de VMs allumées. |
| `vmware_cluster_vsan_enabled` | gauge | cluster, datacenter | 1 si vSAN est activé sur le cluster. |
| `vmware_cluster_dpm_enabled` (*) | gauge | datacenter, cluster | 1 si DPM (gestion d'énergie) est activé. |
| `vmware_cluster_drs_score` (*) | gauge | datacenter, cluster | Score DRS du cluster (0-100, santé des VMs). |
| `vmware_cluster_ha_failover_cpu_resources_configured_percent` (*) | gauge | datacenter, cluster | Pourcentage CPU réservé au basculement HA (configuré ou calculé). |
| `vmware_cluster_ha_failover_cpu_resources_percent` (*) | gauge | datacenter, cluster | Ressources CPU de basculement HA disponibles (%). |
| `vmware_cluster_ha_failover_level_configured` (*) | gauge | datacenter, cluster | Nombre de pannes d'ESX à tolérer configuré (politique 'nombre d'ESX'). |
| `vmware_cluster_ha_failover_level_current` (*) | gauge | datacenter, cluster | Nombre de pannes d'ESX actuellement tolérées (politique 'nombre d'ESX', slots). |
| `vmware_cluster_ha_failover_memory_resources_configured_percent` (*) | gauge | datacenter, cluster | Pourcentage mémoire réservé au basculement HA (configuré ou calculé). |
| `vmware_cluster_ha_failover_memory_resources_percent` (*) | gauge | datacenter, cluster | Ressources mémoire de basculement HA disponibles (%). |

## ESX

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_host_boot_timestamp_seconds` | gauge | cluster, datacenter, host | Horodatage du dernier démarrage de l'ESX. |
| `vmware_host_certificate_expiry_timestamp_seconds` | gauge | cluster, datacenter, host | Date d'expiration du certificat de l'ESX (epoch). |
| `vmware_host_config_issues` | gauge | cluster, datacenter, host | Nombre de problèmes de configuration signalés sur l'ESX. |
| `vmware_host_connection_state` | gauge | cluster, datacenter, host | État de connexion de l'ESX au vCenter (0=disconnected, 1=connected, 2=notResponding). |
| `vmware_host_cpu_cores` | gauge | cluster, datacenter, host | Nombre de coeurs physiques. |
| `vmware_host_cpu_threads` | gauge | cluster, datacenter, host | Nombre de threads physiques. |
| `vmware_host_cpu_total_mhz` | gauge | cluster, datacenter, host | Capacité CPU totale (MHz). |
| `vmware_host_cpu_usage_mhz` | gauge | cluster, datacenter, host | CPU consommé (MHz). |
| `vmware_host_datastores` | gauge | cluster, datacenter, host | Nombre de datastores montés sur l'ESX. |
| `vmware_host_hardware_status` | gauge | cluster, component, datacenter, host, name | Statut d'un composant matériel (mémoire, cpu, stockage) (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_host_info` | gauge | bios_version, build, cluster, cpu_model, datacenter, host, model, product, vendor, version | Informations sur l'ESX (valeur toujours 1). |
| `vmware_host_maintenance_mode` | gauge | cluster, datacenter, host | 1 si l'ESX est en mode maintenance. |
| `vmware_host_memory_total_bytes` | gauge | cluster, datacenter, host | Mémoire physique totale. |
| `vmware_host_memory_usage_bytes` | gauge | cluster, datacenter, host | Mémoire consommée. |
| `vmware_host_overall_status` | gauge | cluster, datacenter, host | Statut global de l'ESX (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_host_pnic_link_up` | gauge | cluster, datacenter, driver, host, nic, used | 1 si le lien de la carte réseau physique est actif. used=true si elle est uplink d'un vSwitch/vDS. |
| `vmware_host_pnic_speed_mbps` | gauge | cluster, datacenter, driver, host, nic, used | Vitesse négociée de la carte réseau physique (Mb/s, 0 si lien down). |
| `vmware_host_power_state` | gauge | cluster, datacenter, host | État d'alimentation de l'ESX (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_host_quarantine_mode` | gauge | cluster, datacenter, host | 1 si l'ESX est en quarantaine (Proactive HA). |
| `vmware_host_reboot_required` | gauge | cluster, datacenter, host | 1 si un redémarrage de l'ESX est requis. |
| `vmware_host_sensor_health` | gauge | cluster, datacenter, host, sensor, type | Santé d'un capteur matériel IPMI/CIM (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_host_sensor_value` | gauge | cluster, datacenter, host, sensor, type, unit | Valeur courante d'un capteur matériel (unité dans le label unit). |
| `vmware_host_service_running` | gauge | cluster, datacenter, host, policy, service | 1 si le service ESX est démarré (TSM-SSH, ntpd, ...). |
| `vmware_host_storage_luns` | gauge | cluster, datacenter, host | Nombre de LUN vus par l'ESX. |
| `vmware_host_storage_luns_degraded` | gauge | cluster, datacenter, host | LUN multi-chemins n'ayant plus qu'un seul chemin vivant (perte de redondance). |
| `vmware_host_storage_luns_no_path` | gauge | cluster, datacenter, host | LUN sans aucun chemin vivant (perte d'accès). |
| `vmware_host_storage_paths` | gauge | cluster, datacenter, host, state | Nombre de chemins de stockage par état (active, standby, dead...). |
| `vmware_host_uptime_seconds` | gauge | cluster, datacenter, host | Uptime de l'ESX. |
| `vmware_host_vms` | gauge | cluster, datacenter, host | Nombre de VMs enregistrées sur l'ESX. |
| `vmware_host_ha_state` (*) | gauge | datacenter, cluster, host, state | État de l'agent HA (FDM) de l'ESX, valeur 1 pour l'état courant. |

## Machines virtuelles

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_vm_connection_state` | gauge | datacenter, vm, vm_moid | État de connexion de la VM (0=connected, 1=disconnected, 2=orphaned, 3=inaccessible, 4=invalid). |
| `vmware_vm_consolidation_needed` | gauge | datacenter, vm, vm_moid | 1 si les disques de la VM doivent être consolidés. |
| `vmware_vm_cpu_demand_mhz` | gauge | datacenter, vm, vm_moid | Demande CPU (MHz). |
| `vmware_vm_cpu_max_mhz` | gauge | datacenter, vm, vm_moid | CPU maximum utilisable (MHz). |
| `vmware_vm_cpu_usage_mhz` | gauge | datacenter, vm, vm_moid | CPU consommé (MHz). |
| `vmware_vm_disks` | gauge | datacenter, vm, vm_moid | Nombre de disques virtuels. |
| `vmware_vm_guest_heartbeat_status` | gauge | datacenter, vm, vm_moid | Heartbeat VMware Tools du système invité (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_vm_info` | gauge | cluster, datacenter, guest_hostname, guest_os, host, hw_version, ip_address, managed_by, template, tools_version, vm, vm_moid | Informations sur la VM (valeur toujours 1). Porte cluster et host courants. |
| `vmware_vm_memory_ballooned_bytes` | gauge | datacenter, vm, vm_moid | Mémoire récupérée par ballooning. |
| `vmware_vm_memory_compressed_bytes` | gauge | datacenter, vm, vm_moid | Mémoire compressée. |
| `vmware_vm_memory_guest_active_bytes` | gauge | datacenter, vm, vm_moid | Mémoire active de l'invité. |
| `vmware_vm_memory_host_consumed_bytes` | gauge | datacenter, vm, vm_moid | Mémoire consommée sur l'ESX. |
| `vmware_vm_memory_size_bytes` | gauge | datacenter, vm, vm_moid | Mémoire configurée. |
| `vmware_vm_memory_swapped_bytes` | gauge | datacenter, vm, vm_moid | Mémoire swappée par l'hyperviseur. |
| `vmware_vm_nics` | gauge | datacenter, vm, vm_moid | Nombre de cartes réseau virtuelles. |
| `vmware_vm_overall_status` | gauge | datacenter, vm, vm_moid | Statut global de la VM (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_vm_power_state` | gauge | datacenter, vm, vm_moid | État d'alimentation de la VM (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_vm_question_pending` | gauge | datacenter, vm, vm_moid | 1 si la VM est bloquée sur une question en attente de réponse. |
| `vmware_vm_snapshots` | gauge | datacenter, vm, vm_moid | Nombre de snapshots de la VM. |
| `vmware_vm_storage_committed_bytes` | gauge | datacenter, vm, vm_moid | Stockage réellement consommé sur les datastores. |
| `vmware_vm_storage_uncommitted_bytes` | gauge | datacenter, vm, vm_moid | Stockage provisionné non encore consommé (thin). |
| `vmware_vm_tools_running` | gauge | datacenter, vm, vm_moid | 1 si VMware Tools tourne dans l'invité. |
| `vmware_vm_tools_version_status` | gauge | datacenter, status, vm, vm_moid | Statut de version des VMware Tools (label status, valeur 1). |
| `vmware_vm_uptime_seconds` | gauge | datacenter, vm, vm_moid | Uptime de la VM. |
| `vmware_vm_vcpus` | gauge | datacenter, vm, vm_moid | Nombre de vCPU. |
| `vmware_vms_monitored` | gauge |  | Nombre de VMs supervisées (après filtres). |
| `vmware_vms_powered_on` | gauge |  | Nombre de VMs supervisées allumées. |
| `vmware_vms_system_managed` | gauge |  | Nombre de VMs gérées par une solution (vCLS, Supervisor, agents...). |
| `vmware_vms_templates` | gauge |  | Nombre de templates dans l'inventaire. |
| `vmware_vm_boot_timestamp_seconds` (*) | gauge | datacenter, vm, vm_moid | Horodatage du dernier démarrage de la VM. |
| `vmware_vm_guest_disk_capacity_bytes` (*) | gauge | datacenter, vm, vm_moid, disk_path | Capacité d'un système de fichiers de l'invité (via VMware Tools). |
| `vmware_vm_guest_disk_free_bytes` (*) | gauge | datacenter, vm, vm_moid, disk_path | Espace libre d'un système de fichiers de l'invité (via VMware Tools). |
| `vmware_vm_ha_protected` (*) | gauge | datacenter, vm, vm_moid | 1 si la VM est protégée par vSphere HA. |
| `vmware_vm_snapshot_oldest_age_seconds` (*) | gauge | datacenter, vm, vm_moid | Âge du plus ancien snapshot de la VM. |
| `vmware_vm_snapshot_oldest_timestamp_seconds` (*) | gauge | datacenter, vm, vm_moid | Date de création du plus ancien snapshot. |

## Datastores

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_datastore_accessible` | gauge | datacenter, datastore, datastore_moid | 1 si le datastore est accessible. |
| `vmware_datastore_capacity_bytes` | gauge | datacenter, datastore, datastore_moid | Capacité du datastore. |
| `vmware_datastore_cluster_capacity_bytes` | gauge | datacenter, datastore_cluster | Capacité du cluster de datastores. |
| `vmware_datastore_cluster_datastores` | gauge | datacenter, datastore_cluster | Nombre de datastores dans le cluster de datastores. |
| `vmware_datastore_cluster_free_bytes` | gauge | datacenter, datastore_cluster | Espace libre du cluster de datastores. |
| `vmware_datastore_cluster_overall_status` | gauge | datacenter, datastore_cluster | Statut global du cluster de datastores (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_free_bytes` | gauge | datacenter, datastore, datastore_moid | Espace libre du datastore. |
| `vmware_datastore_hosts_inaccessible` | gauge | datacenter, datastore, datastore_moid | Nombre d'ESX pour lesquels le datastore est inaccessible (APD/PDL...). |
| `vmware_datastore_hosts_mounted` | gauge | datacenter, datastore, datastore_moid | Nombre d'ESX montant le datastore. |
| `vmware_datastore_info` | gauge | datacenter, datastore, datastore_cluster, datastore_moid, shared, type, url | Informations sur le datastore (valeur toujours 1). |
| `vmware_datastore_maintenance_mode` | gauge | datacenter, datastore, datastore_moid | 1 si le datastore est en mode maintenance (ou en entrée). |
| `vmware_datastore_overall_status` | gauge | datacenter, datastore, datastore_moid | Statut global du datastore (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_provisioned_bytes` | gauge | datacenter, datastore, datastore_moid | Espace provisionné total (utilisé + thin non consommé). |
| `vmware_datastore_uncommitted_bytes` | gauge | datacenter, datastore, datastore_moid | Espace provisionné non consommé (thin) sur le datastore. |
| `vmware_datastore_vms` | gauge | datacenter, datastore, datastore_moid | Nombre de VMs ayant des fichiers sur le datastore. |

## vSphere Distributed Switch

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_dvs_hosts` | gauge | datacenter, dvs | Nombre d'ESX membres du vDS. |
| `vmware_dvs_info` | gauge | datacenter, dvs, version | Informations sur le vDS (valeur toujours 1). |
| `vmware_dvs_overall_status` | gauge | datacenter, dvs | Statut global du vDS (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_dvs_ports` | gauge | datacenter, dvs | Nombre de ports du vDS. |
| `vmware_dvs_host_member_up` (*) | gauge | datacenter, dvs, host | 1 si l'ESX membre du vDS est 'up' (sinon outOfSync, warning, down...). |
| `vmware_dvs_host_members` (*) | gauge | datacenter, dvs, status | Nombre d'ESX membres du vDS par statut. |

## Réseaux / portgroups

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_network_accessible` | gauge | datacenter, network, network_moid, network_type | 1 si le réseau est accessible depuis au moins un ESX. |
| `vmware_network_hosts` | gauge | datacenter, network, network_moid, network_type | Nombre d'ESX ayant accès au réseau. |
| `vmware_network_overall_status` | gauge | datacenter, network, network_moid, network_type | Statut global du réseau / portgroup (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_network_vms` | gauge | datacenter, network, network_moid, network_type | Nombre de VMs connectées au réseau. |

## Resource pools

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_resourcepool_cpu_limit_mhz` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Limite CPU du pool (MHz, -1 = illimité). |
| `vmware_resourcepool_cpu_reservation_mhz` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Réservation CPU du pool (MHz). |
| `vmware_resourcepool_memory_limit_bytes` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Limite mémoire du pool (-1 = illimité). |
| `vmware_resourcepool_memory_reservation_bytes` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Réservation mémoire du pool. |
| `vmware_resourcepool_overall_status` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Statut global du resource pool (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_resourcepool_vms` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Nombre de VMs directement dans le resource pool. |
| `vmware_resourcepool_cpu_usage_mhz` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | CPU consommé par le resource pool (MHz). |
| `vmware_resourcepool_memory_ballooned_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Mémoire en ballooning dans le pool. |
| `vmware_resourcepool_memory_guest_active_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Mémoire active des invités du pool. |
| `vmware_resourcepool_memory_host_consumed_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Mémoire consommée sur les ESX par le pool. |
| `vmware_resourcepool_memory_swapped_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Mémoire swappée dans le pool. |

## vSAN

| Métrique | Type | Labels | Description |
|---|---|---|---|
| `vmware_vsan_cluster_health` (*) | gauge | datacenter, cluster | Santé globale vSAN du cluster (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_group` (*) | gauge | datacenter, cluster, group | Santé d'un groupe de tests vSAN (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_query_success` (*) | gauge | datacenter, cluster | 1 si le résumé de santé vSAN a pu être lu pour le cluster. |
| `vmware_vsan_health_test` (*) | gauge | datacenter, cluster, group, test | Résultat d'un test de santé vSAN (0=gray/inconnu, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_timestamp_seconds` (*) | gauge | datacenter, cluster | Horodatage du dernier test de santé vSAN. |

