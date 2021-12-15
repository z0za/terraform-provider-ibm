// Copyright IBM Corp. 2021 All Rights Reserved.
// Licensed under the Mozilla Public License v2.0

package ibm

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceIBMSatconClusterGroup() *schema.Resource {
	return &schema.Resource{
		Create:   resourceIBMSatconClusterGroupCreate,
		Read:     resourceIBMSatconClusterGroupRead,
		Update:   resourceIBMSatconClusterGroupUpdate,
		Delete:   resourceIBMSatconClusterGroupDelete,
		Importer: &schema.ResourceImporter{},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"uuid": {
				Description: "ID of the clustergroup",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"name": {
				Description: "Name or id of the clustergroup",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"created": {
				Description: "Creation time of the clustergroup",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"clusters": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cluster_id": {
							Description: "ID of the cluster",
							Type:        schema.TypeString,
							Required:    true,
						},
						"name": {
							Description: "Name of the cluster",
							Type:        schema.TypeString,
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func resourceIBMSatconClusterGroupCreate(d *schema.ResourceData, meta interface{}) error {
	satconClient, err := meta.(ClientSession).SatellitConfigClientSession()
	if err != nil {
		return err
	}

	satconGroupAPI := satconClient.Groups

	userDetails, err := meta.(ClientSession).BluemixUserDetails()
	if err != nil {
		return err
	}

	name := d.Get("name").(string)
	log.Printf("[DEBUG] create clustergroup with name: %s , userid: %v\n", name, userDetails.userAccount)
	addDetails, err := satconGroupAPI.AddGroup(userDetails.userAccount, name)
	if err != nil {
		log.Printf("[DEBUG] resourceIBMSatconClusterGroupCreate AddGroup failed with: %v\n", err)
		return fmt.Errorf("error creating satellite clustergroup: %s", err)
	}

	d.SetId(name)
	d.Set("uuid", addDetails.UUID)

	//TODO Should we wait for clusters to be able to attach to group?

	if clusters, ok := d.GetOk("clusters"); ok {
		newClusters := clusters.([]interface{})
		clusterIDsForAttach := make([]string, 0)
		for _, nCl := range newClusters {
			newCluster := nCl.(map[string]interface{})
			clusterID := newCluster["cluster_id"].(string)
			if clusterID == "" {
				log.Printf("[DEBUG] resourceIBMSatconClusterGroupCreate cluster id was empty, skip attach: %v\n", newCluster)
				continue
			}
			clusterIDsForAttach = append(clusterIDsForAttach, clusterID)
		}
		if len(clusterIDsForAttach) > 0 {
			_, err := satconGroupAPI.GroupClusters(userDetails.userAccount, addDetails.UUID, clusterIDsForAttach)
			if err != nil {
				log.Printf("[DEBUG] resourceIBMSatconClusterGroupCreate GroupClusters failed with: %v\n", err)
				return fmt.Errorf("error attaching satellite clustergroup: %s", err)
			}

		}
	}

	return resourceIBMSatconClusterGroupRead(d, meta)
}

func resourceIBMSatconClusterGroupRead(d *schema.ResourceData, meta interface{}) error {
	groupName := d.Id()

	if groupName == "" {
		return fmt.Errorf("satellite clustergroup name is empty")
	}

	satconClient, err := meta.(ClientSession).SatellitConfigClientSession()
	if err != nil {
		return err
	}

	satconGroupAPI := satconClient.Groups

	userDetails, err := meta.(ClientSession).BluemixUserDetails()
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] get clustergroup with name: %s , userid: %v\n", name, userDetails.userAccount)

	group, err := satconGroupAPI.GroupByName(userDetails.userAccount, groupName)
	if err != nil {
		return fmt.Errorf("error retrieving satellite clustergroup: %s", err)
	}

	clusters := make([]map[string]interface{}, 0)
	for _, c := range group.Clusters {
		cluster := map[string]interface{}{
			"cluster_id": c.ID,
			"name":       c.Name,
		}
		clusters = append(clusters, cluster)
	}

	d.Set("uuid", group.UUID)
	d.Set("name", group.Name)
	d.Set("created", group.Created)
	d.Set("clusters", clusters)

	return nil
}

func resourceIBMSatconClusterGroupUpdate(d *schema.ResourceData, meta interface{}) error {
	if !d.HasChange("clusters") {
		log.Printf("[DEBUG] resourceIBMSatconClusterGroupUpdate no change in clusters field")
		return nil
	}
	groupName := d.Id()
	uuid := d.Get("uuid").(string)

	if groupName == "" {
		return fmt.Errorf("satellite clustergroup name is empty")
	}

	satconClient, err := meta.(ClientSession).SatellitConfigClientSession()
	if err != nil {
		return err
	}

	satconGroupAPI := satconClient.Groups

	userDetails, err := meta.(ClientSession).BluemixUserDetails()
	if err != nil {
		return err
	}

	clusterIDsForAttach := make([]string, 0)
	clusterIDsForDetach := make([]string, 0)

	oldClustersIf, newClustersIf := d.GetChange("clusters")
	oldClusters := oldClustersIf.([]interface{})
	newClusters := newClustersIf.([]interface{})
	for _, nCl := range newClusters {
		newCluster := nCl.(map[string]interface{})
		exists := false
		for _, oCl := range oldClusters {
			oldCluster := oCl.(map[string]interface{})
			if strings.Compare(newCluster["cluster_id"].(string), oldCluster["cluster_id"].(string)) == 0 {
				exists = true
				break
			}
		}
		if !exists {
			//need to attach the new cluster
			clusterIDsForAttach = append(clusterIDsForAttach, newCluster["cluster_id"].(string))
		}
	}
	for _, oCl := range oldClusters {
		oldCluster := oCl.(map[string]interface{})
		exists := false
		for _, nCl := range newClusters {
			newCluster := nCl.(map[string]interface{})

			if strings.Compare(newCluster["cluster_id"].(string), oldCluster["cluster_id"].(string)) == 0 {
				exists = true
				break
			}
		}
		if !exists {
			//need to detach the old cluster
			clusterIDsForDetach = append(clusterIDsForDetach, oldCluster["cluster_id"].(string))
		}
	}

	if len(clusterIDsForAttach) > 0 {
		_, err := satconGroupAPI.GroupClusters(userDetails.userAccount, uuid, clusterIDsForAttach)
		if err != nil {
			log.Printf("[DEBUG] resourceIBMSatconClusterGroupUpdate GroupClusters failed with: %v\n", err)
			return fmt.Errorf("error attaching satellite clustergroup: %s", err)
		}

	}

	if len(clusterIDsForDetach) > 0 {
		_, err := satconGroupAPI.UnGroupClusters(userDetails.userAccount, uuid, clusterIDsForDetach)
		if err != nil {
			log.Printf("[DEBUG] resourceIBMSatconClusterGroupUpdate UnGroupClusters failed with: %v\n", err)
			return fmt.Errorf("error detaching satellite clustergroup: %s", err)
		}

	}

	return nil
}

func resourceIBMSatconClusterGroupDelete(d *schema.ResourceData, meta interface{}) error {
	groupName := d.Id()

	if groupName == "" {
		return fmt.Errorf("satellite clustergroup name is empty")
	}

	satconClient, err := meta.(ClientSession).SatellitConfigClientSession()
	if err != nil {
		return err
	}

	satconGroupAPI := satconClient.Groups

	userDetails, err := meta.(ClientSession).BluemixUserDetails()
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] remove clustergroup with name: %s , userid: %v\n", name, userDetails.userAccount)

	removeDetails, err := satconGroupAPI.RemoveGroupByName(userDetails.userAccount, groupName)
	if err != nil {
		return fmt.Errorf("failed deleting satellite clustergroup: %s", err)
	}

	log.Printf("[INFO] Removed satellite clustergroup with name: %s, uuid: %s", groupName, removeDetails.UUID)

	d.SetId("")
	return nil
}
