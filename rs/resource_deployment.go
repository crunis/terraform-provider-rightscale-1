package rs

import (
	"errors"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/rightscale/terraform-provider-rs/rs/rsc"
)

var deploymentSchema = map[string]*schema.Schema{
	"name": &schema.Schema{
		Description: "name of deployment",
		Type:        schema.TypeString,
		Required:    true,
	},
	"description": &schema.Schema{
		Description: "description of deployment",
		Type:        schema.TypeString,
		Optional:    true,
	},
	"resource_group_href": &schema.Schema{
		Description: "href of the Windows Azure Resource Group attached to the deployment",
		Type:        schema.TypeString,
		Optional:    true,
	},
	"locked": &schema.Schema{
		Description: "whether deployment is locked",
		Type:        schema.TypeBool,
		Optional:    true,
	},
	"server_tag_scope": &schema.Schema{
		Description:  "routing scope for tags for servers in the deployment",
		Type:         schema.TypeString,
		Optional:     true,
		InputDefault: "deployment",
		ForceNew:     true,
		ValidateFunc: func(v interface{}, _ string) (warns []string, errs []error) {
			if v == "" || v == "account" || v == "deployment" {
				return nil, nil
			}
			return nil, []error{errors.New(`server_tag_scope must be "account" or "deployment"`)}
		},
	},
}

func resourceDeployment() *schema.Resource {
	return &schema.Resource{
		Schema: deploymentSchema,
		Read:   resourceRead,
		Exists: resourceExists,
		Delete: resourceDelete, // can fail if deployment is locked - that's what we want
		Create: resourceDeploymentCreate,
		Update: resourceDeploymentUpdate,
	}
}

func resourceDeploymentCreate(d *schema.ResourceData, m interface{}) error {
	var mustLock bool
	{
		locked, ok := d.GetOk("locked")
		mustLock = ok && locked.(bool)
	}

	client := m.(rsc.Client)
	res, err := client.Create("rs_cm", "deployment", deploymentFields(d))
	if err != nil {
		return err
	}
	for k, v := range res.Fields {
		d.Set(k, v)
	}

	if mustLock {
		if err := updateLock(d, client); err != nil {
			// Attempt to delete previously created deployment, ignore errors
			client.Delete(res.Locator)
			return err
		}
		d.Set("locked", true)
	}

	// set ID last so Terraform does not assume the deployment has been
	// created until all operations have completed successfully.
	d.SetId(res.Locator.Namespace + ":" + res.Locator.Href)
	return nil
}

func resourceDeploymentUpdate(d *schema.ResourceData, m interface{}) error {
	d.Partial(true)

	client := m.(rsc.Client)
	loc, err := locator(d)
	if err != nil {
		return err
	}

	// update lock
	if err := updateLock(d, client); err != nil {
		return handleError(d, err)
	}
	d.SetPartial("locked")

	// then the other fields
	if err := client.Update(loc, deploymentFields(d)); err != nil {
		return handleError(d, err)
	}

	d.Partial(false)
	return nil
}

// updateLock is a helper function that takes care of locking or unlocking the
// deployment according to the value of the "locked" resource data field.
func updateLock(d *schema.ResourceData, client rsc.Client) error {
	loc, err := locator(d)
	if err != nil {
		return err
	}
	lock := d.Get("locked").(bool)
	if lock {
		return client.Run(loc, "@res.lock()")
	}
	return client.Run(loc, "@res.unlock()")
}

func deploymentFields(d *schema.ResourceData) rsc.Fields {
	fields := rsc.Fields{"name": d.Get("name")}
	if desc, ok := d.GetOk("description"); ok {
		fields["description"] = desc
	}
	if rghref, ok := d.GetOk("resource_group_href"); ok {
		fields["resource_group_href"] = rghref
	}
	if scope, ok := d.GetOk("server_tag_scope"); ok {
		fields["server_tag_scope"] = scope
	}
	return fields
}