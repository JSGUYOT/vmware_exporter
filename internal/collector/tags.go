package collector

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/JSGUYOT/vmware_exporter/internal/vsphere"
)

const taggingAPI = "/api/cis/tagging"

// vmTag is one tag attached to a VM.
type vmTag struct{ category, tag string }

// tagCache keeps the last tags read successfully, so that a failed read does
// not make the VMs lose their tags (rules joining on them would go silent).
type tagCache struct {
	byVM map[string][]vmTag
	read time.Time
}

// collectTags emits the tags of the monitored VMs, for the configured categories.
func collectTags(ctx context.Context, r *Run) error {
	var err error
	if r.tags.byVM == nil || time.Since(r.tags.read) >= r.Cfg.Tags.RefreshInterval {
		err = r.refreshTags(ctx)
	}
	ok, last := 1.0, 0.0
	if err != nil {
		ok = 0
	}
	if !r.tags.read.IsZero() {
		last = float64(r.tags.read.Unix())
	}
	s := r.Sink
	s.Gauge(n("tags_read_success"), "1 if the last read of the vSphere tags succeeded.", ok, nil)
	s.Gauge(n("tags_last_read_timestamp_seconds"), "Timestamp of the last successful read of the vSphere tags (0 = never).", last, nil)
	for moid, tags := range r.tags.byVM {
		l, monitored := r.vmLabels[moid]
		if !monitored {
			continue
		}
		for _, t := range tags {
			s.Gauge(n("vm_tag"), "vSphere tag attached to the VM (value always 1).", 1, append(l[:len(l):len(l)], "category", t.category, "tag", t.tag))
		}
	}
	return err
}

func (r *Run) refreshTags(ctx context.Context) error {
	rc, err := r.Client.REST(ctx)
	if err != nil {
		return err
	}
	byVM, err := readVMTags(ctx, rc, r.Cfg.Tags.Categories)
	if err != nil {
		return fmt.Errorf("reading vSphere tags: %w", err)
	}
	r.tags.byVM, r.tags.read = byVM, time.Now()
	return nil
}

// gone reports an object that was deleted since it was listed, or that the
// account may not read: it is skipped, and a wanted category ends up missing.
func gone(err error) bool {
	return errors.Is(err, vsphere.ErrNotFound) || errors.Is(err, vsphere.ErrForbidden)
}

// readVMTags returns the tags of the given categories by VM moid. A category
// that cannot be found is an error: the Automation API omits what the account
// may not read, which must not look like "no VM has this tag".
func readVMTags(ctx context.Context, rc *vsphere.RESTClient, categories []string) (map[string][]vmTag, error) {
	var ids []string
	if err := rc.Get(ctx, taggingAPI+"/category", &ids); err != nil {
		return nil, err
	}
	names := map[string]vmTag{} // tag id -> category and tag names
	found := map[string]bool{}
	for _, id := range ids {
		var cat struct {
			Name string `json:"name"`
		}
		if err := rc.Get(ctx, taggingAPI+"/category/"+url.PathEscape(id), &cat); err != nil {
			if gone(err) {
				continue
			}
			return nil, err
		}
		category := ""
		for _, c := range categories {
			if strings.EqualFold(c, cat.Name) { // vCenter category names ignore case
				category = c
			}
		}
		if category == "" {
			continue
		}
		found[category] = true
		var tagIDs []string
		body := map[string]string{"category_id": id}
		if err := rc.Post(ctx, taggingAPI+"/tag?action=list-tags-for-category", body, &tagIDs); err != nil {
			return nil, err
		}
		for _, tid := range tagIDs {
			var tag struct {
				Name string `json:"name"`
			}
			if err := rc.Get(ctx, taggingAPI+"/tag/"+url.PathEscape(tid), &tag); err != nil {
				if gone(err) {
					continue
				}
				return nil, err
			}
			names[tid] = vmTag{category, tag.Name}
		}
	}
	for _, c := range categories {
		if !found[c] {
			return nil, fmt.Errorf("tag category %q not found (missing, or not readable by this account)", c)
		}
	}
	byVM := map[string][]vmTag{}
	if len(names) == 0 {
		return byVM, nil
	}
	tagIDs := make([]string, 0, len(names))
	for id := range names {
		tagIDs = append(tagIDs, id)
	}
	var attached []struct {
		TagID   string `json:"tag_id"`
		Objects []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"object_ids"`
	}
	body := map[string][]string{"tag_ids": tagIDs}
	if err := rc.Post(ctx, taggingAPI+"/tag-association?action=list-attached-objects-on-tags", body, &attached); err != nil {
		return nil, err
	}
	for _, a := range attached {
		for _, o := range a.Objects {
			if o.Type == "VirtualMachine" {
				byVM[o.ID] = append(byVM[o.ID], names[a.TagID])
			}
		}
	}
	return byVM, nil
}
